$PORTAL = "https://fw.bits-pilani.ac.in:8090"
$CHECK_URL = "http://connectivitycheck.gstatic.com/generate_204"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoDir = Split-Path -Parent $ScriptDir
$CredsFile = Join-Path $RepoDir "creds.conf"
$LogFile = Join-Path $RepoDir "fortinet-login.log"
$RunId = "{0}_{1}" -f $env:USERNAME, $PID
$CookieFile = Join-Path $env:TEMP "fortinet_cookies_$RunId.txt"
$DebugFile = Join-Path $env:TEMP "fortinet_debug_$RunId.html"

# TLS verification is ON by default -- credentials are POSTed to the portal.
# If the portal certificate is not trusted by this machine, set BITS_INSECURE=1.
# ponytail: env escape hatch beats pinning; swap for --cacert if a CA ever ships.
$TlsOpt = if ($env:BITS_INSECURE -eq '1') { @('-k') } else { @() }

function Log {
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingWriteHost', '')]
    param([string]$Message)

    $line = "[{0}] {1}" -f (Get-Date -Format 'HH:mm:ss'), $Message
    Write-Host $line
    try {
        Add-Content -Path $LogFile -Value $line -Encoding UTF8
    } catch {
        # Logging should not block authentication attempts.
        Write-Debug $_.Exception.Message
    }
}

function Unescape-CredsValue {
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseApprovedVerbs', '')]
    param([string]$Value)

    $builder = New-Object System.Text.StringBuilder

    for ($i = 0; $i -lt $Value.Length; $i++) {
        $char = $Value[$i]
        if ($char -eq '\' -and $i + 1 -lt $Value.Length) {
            $i++
            switch ($Value[$i]) {
                '\' { [void]$builder.Append('\') }
                '"' { [void]$builder.Append('"') }
                'n' { [void]$builder.Append("`n") }
                'r' { [void]$builder.Append("`r") }
                't' { [void]$builder.Append("`t") }
                default { [void]$builder.Append($Value[$i]) }
            }
            continue
        }

        [void]$builder.Append($char)
    }

    return $builder.ToString()
}

function Get-CredsValue {
    param(
        [string]$RawContent,
        [string]$Key
    )

    $pattern = "(?m)^\s*{0}\s*=\s*(?:""((?:\\.|[^""])*)""|([^\r\n]+?))\s*$" -f [regex]::Escape($Key)
    $match = [regex]::Match($RawContent, $pattern)
    if (-not $match.Success) {
        return $null
    }

    if ($match.Groups[1].Success) {
        return Unescape-CredsValue $match.Groups[1].Value
    }

    return $match.Groups[2].Value.Trim()
}

function Remove-TempFile {
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseShouldProcessForStateChangingFunctions', '')]
    param()
    Remove-Item $CookieFile -Force -ErrorAction SilentlyContinue
}

# -- Load Credentials --------------------------------------------------------
if (-not (Test-Path $CredsFile)) {
    Log "ERROR: Credentials file not found at $CredsFile"
    exit 1
}

$creds = (Get-Content $CredsFile -Raw -Encoding UTF8).TrimStart([char]0xFEFF)
$script:USERNAME = Get-CredsValue -RawContent $creds -Key "USERNAME"
$script:PASSWORD = Get-CredsValue -RawContent $creds -Key "PASSWORD"

if ([string]::IsNullOrWhiteSpace($script:USERNAME) -or [string]::IsNullOrWhiteSpace($script:PASSWORD)) {
    Log "ERROR: creds.conf is missing USERNAME or PASSWORD."
    exit 1
}

# -- Functions ----------------------------------------------------------------

function Get-CurrentSsid {
    $output = netsh wlan show interfaces 2>$null
    foreach ($line in $output) {
        if ($line -match '^\s*SSID\s+:\s+(.+)$' -and $line -notmatch '^\s*BSSID\s+:') {
            return $Matches[1].Trim()
        }
    }

    return "Unknown"
}

function Write-TlsHint {
    if ($env:BITS_INSECURE -eq '1') { return }
    Log "  If the portal certificate is untrusted, retry with BITS_INSECURE=1 (disables TLS verification)."
}

function Test-BitsSsid {
    param([string]$Ssid)

    return $Ssid -match '^BITS-(STUDENT|STAFF)$'
}

function Test-LoggedIn {
    $code = curl.exe -s @TlsOpt --max-time 5 -o NUL -w "%{http_code}" $CHECK_URL
    Log "Check: Connectivity status code is $code"
    return ($code -eq "204")
}

function Get-MagicToken {
    $redirect = curl.exe -s @TlsOpt --max-time 10 -o NUL -w "%{redirect_url}" $CHECK_URL
    Log "Debug: Redirect URL received: $redirect"
    if ($redirect -match 'magic=([a-f0-9]+)') { return $Matches[1] }
    if ($redirect -match 'fgtauth\?([a-f0-9]+)') { return $Matches[1] }

    Log "Debug: No redirect header found, checking HTML body..."
    $body = curl.exe -s @TlsOpt --max-time 10 $CHECK_URL
    if ($body -match 'magic=([a-f0-9]+)') { return $Matches[1] }
    if ($body -match 'fgtauth\?([a-f0-9]+)') { return $Matches[1] }

    Log "Debug: Still no magic token, querying portal directly..."
    $portalBody = curl.exe -s @TlsOpt --max-time 10 "${PORTAL}/"
    if ($portalBody -match 'name="magic"\s+value="([a-f0-9]+)"') {
        return $Matches[1]
    }

    return $null
}

function Login {
    $magic = Get-MagicToken
    if (-not $magic) {
        Log "Could not get magic token -- captive portal not detected (already logged in, or portal unreachable)."
        Write-TlsHint
        return $false
    }
    Log "Got magic token: $magic"

    Log "Step 1: Initializing session via fgtauth..."
    curl.exe -c "$CookieFile" -b "$CookieFile" -sL @TlsOpt "${PORTAL}/fgtauth?${magic}" -o NUL

    Log "Step 2: Submitting credentials for user $script:USERNAME..."
    $postBodyFile = Join-Path $env:TEMP "fortinet_body_$RunId.tmp"
    $httpCode = curl.exe -c "$CookieFile" -b "$CookieFile" -s @TlsOpt -X POST "${PORTAL}/" `
        --data-urlencode "username=$script:USERNAME" `
        --data-urlencode "password=$script:PASSWORD" `
        --data "magic=$magic" `
        --data "4Tredir=http://connectivitycheck.gstatic.com/generate_204" `
        -o $postBodyFile `
        -w '%{http_code}'
    $postResponse = (Get-Content $postBodyFile -Raw -Encoding UTF8).TrimStart([char]0xFEFF)
    Remove-Item $postBodyFile -Force -ErrorAction SilentlyContinue
    Log "Portal responded: HTTP $httpCode"

    if ($postResponse -match 'keepalive\?([a-f0-9]+)') {
        $keepalive = $Matches[1]
        Log "Credentials accepted. Keepalive token: $keepalive"

        Log "Step 3: Activating connection..."
        curl.exe -c "$CookieFile" -b "$CookieFile" -sL @TlsOpt "${PORTAL}/keepalive?${keepalive}" -o NUL
    } else {
        $postResponse | Out-File $DebugFile -Encoding UTF8
        if ($postResponse -match 'invalid.{0,30}(credential|password|user)|wrong.{0,20}(password|user)|authentication.{0,20}fail|please.{0,10}try.{0,10}again|login.{0,10}fail') {
            Log "[X] Portal rejected credentials -- wrong username or password."
        } else {
            Log "[X] No keepalive found -- unexpected portal response (HTTP $httpCode)."
            if ($httpCode -eq "000") { Write-TlsHint }
        }
        Log "  Response saved to $DebugFile -- attach when reporting a bug."
    }

    Start-Sleep -Seconds 2
    if (Test-LoggedIn) { return $true }
    Log "[X] Login failed."
    if (Test-Path $DebugFile) { Log "  See $DebugFile for the portal response." }
    return $false
}

# -- Main ---------------------------------------------------------------------

try {
    $ssid = Get-CurrentSsid
    if (-not (Test-BitsSsid -Ssid $ssid)) {
        Log "Current WiFi is '$ssid'; not a BITS network. Skipping."
        exit 0
    }

    Log "Checking connectivity..."
    if (Test-LoggedIn) {
        Log "Already authenticated, nothing to do."
        exit 0
    }

    for ($i = 1; $i -le 2; $i++) {
        Log "Attempt $i/2..."
        if (Login) {
            Log "SUCCESS: Login successful."
            exit 0
        }
        Start-Sleep -Seconds 3
    }

    Log "FAIL: All attempts failed."
    exit 1
} finally {
    Remove-TempFile
}
