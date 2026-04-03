$PORTAL = "https://fw.bits-pilani.ac.in:8090"
$CHECK_URL = "http://connectivitycheck.gstatic.com/generate_204"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoDir = Split-Path -Parent $ScriptDir
$CredsFile = Join-Path $RepoDir "creds.conf"
$LogFile = Join-Path $RepoDir "fortinet-login.log"
$RunId = "{0}_{1}" -f $env:USERNAME, $PID
$CookieFile = Join-Path $env:TEMP "fortinet_cookies_$RunId.txt"
$DebugFile = Join-Path $env:TEMP "fortinet_debug_$RunId.html"

function Log {
    param([string]$Message)

    $line = "[{0}] {1}" -f (Get-Date -Format 'HH:mm:ss'), $Message
    Write-Host $line
    try {
        Add-Content -Path $LogFile -Value $line -Encoding UTF8
    } catch {
        # Logging should not block authentication attempts.
    }
}

function Unescape-CredsValue {
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

function Cleanup-TempFiles {
    Remove-Item $CookieFile -Force -ErrorAction SilentlyContinue
}

# ── Load Credentials ──────────────────────────────────────────────────────────
if (-not (Test-Path $CredsFile)) {
    Log "ERROR: Credentials file not found at $CredsFile"
    exit 1
}

$creds = Get-Content $CredsFile -Raw
$global:USERNAME = Get-CredsValue -RawContent $creds -Key "USERNAME"
$global:PASSWORD = Get-CredsValue -RawContent $creds -Key "PASSWORD"

if ([string]::IsNullOrWhiteSpace($global:USERNAME) -or [string]::IsNullOrWhiteSpace($global:PASSWORD)) {
    Log "ERROR: creds.conf is missing USERNAME or PASSWORD."
    exit 1
}

# ── Functions ─────────────────────────────────────────────────────────────────

function Is-LoggedIn {
    $code = curl.exe -sk --max-time 5 -o NUL -w "%{http_code}" $CHECK_URL
    Log "Check: Connectivity status code is $code"
    return ($code -eq "204")
}

function Get-MagicToken {
    $redirect = curl.exe -sk --max-time 10 -o NUL -w "%{redirect_url}" $CHECK_URL
    Log "Debug: Redirect URL received: $redirect"
    if ($redirect -match 'magic=([a-f0-9]+)') { return $Matches[1] }
    if ($redirect -match 'fgtauth\?([a-f0-9]+)') { return $Matches[1] }

    Log "Debug: No redirect header found, checking HTML body..."
    $body = curl.exe -sk --max-time 10 $CHECK_URL
    if ($body -match 'magic=([a-f0-9]+)') { return $Matches[1] }
    if ($body -match 'fgtauth\?([a-f0-9]+)') { return $Matches[1] }

    Log "Debug: Still no magic token, querying portal directly..."
    $portalBody = curl.exe -sk --max-time 10 "${PORTAL}/"
    if ($portalBody -match 'name="magic"\s+value="([a-f0-9]+)"') {
        return $Matches[1]
    }

    return $null
}

function Login {
    $magic = Get-MagicToken
    if (-not $magic) {
        Log "FAIL: No magic token found. Are you connected to BITS-WiFi?"
        return $false
    }
    Log "SUCCESS: Magic token is $magic"

    Log "Step 1: Initializing session via fgtauth..."
    curl.exe -c "$CookieFile" -b "$CookieFile" -skL "${PORTAL}/fgtauth?${magic}" -o NUL

    Log "Step 2: Submitting credentials for user $global:USERNAME..."
    $postResponse = curl.exe -c "$CookieFile" -b "$CookieFile" -sk -X POST "${PORTAL}/" `
        --data-urlencode "username=$global:USERNAME" `
        --data-urlencode "password=$global:PASSWORD" `
        --data "magic=$magic" `
        --data "4Tredir=http://connectivitycheck.gstatic.com/generate_204"

    if ($postResponse -match 'keepalive\?([a-f0-9]+)') {
        $keepalive = $Matches[1]
        Log "SUCCESS: Credentials accepted. Keepalive token: $keepalive"

        Log "Step 3: Activating connection..."
        curl.exe -c "$CookieFile" -b "$CookieFile" -skL "${PORTAL}/keepalive?${keepalive}" -o NUL
    } else {
        Log "FAIL: No keepalive found in response. Dumping response to $DebugFile"
        $postResponse | Out-File $DebugFile -Encoding UTF8
    }

    Start-Sleep -Seconds 2
    return Is-LoggedIn
}

# ── Main ──────────────────────────────────────────────────────────────────────

try {
    Log "Checking connectivity..."
    if (Is-LoggedIn) {
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
    Cleanup-TempFiles
}
