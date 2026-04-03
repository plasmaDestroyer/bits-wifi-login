$PORTAL = "https://fw.bits-pilani.ac.in:8090"
$CHECK_URL = "http://connectivitycheck.gstatic.com/generate_204"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$CredsFile = Join-Path $ScriptDir "..\creds.conf"
$CookieFile = Join-Path $env:TEMP "fortinet_cookies.txt"

function Log { param($msg) Write-Host "[$(Get-Date -Format 'HH:mm:ss')] $msg" }

# ── Load Credentials ──────────────────────────────────────────────────────────
if (-not (Test-Path $CredsFile)) {
    Log "ERROR: Credentials file not found at $CredsFile"
    exit 1
}

$creds = Get-Content $CredsFile -Raw
if ($creds -match 'USERNAME="?([^"\r\n]+)"?') { $global:USERNAME = $Matches[1] }
if ($creds -match 'PASSWORD="?([^"\r\n]+)"?') { $global:PASSWORD = $Matches[1] }

# ── Functions ─────────────────────────────────────────────────────────────────

function Is-LoggedIn {
    # We use -w "%{http_code}" to see exactly what Google says
    $code = curl.exe -sk --max-time 5 -o NUL -w "%{http_code}" $CHECK_URL
    Log "Check: Connectivity status code is $code"
    return ($code -eq "204")
}

function Get-MagicToken {
    # 1. Try captive portal redirect URL
    $redirect = curl.exe -sk --max-time 10 -o NUL -w "%{redirect_url}" $CHECK_URL
    Log "Debug: Redirect URL received: $redirect"
    if ($redirect -match 'magic=([a-f0-9]+)') { return $Matches[1] }
    if ($redirect -match 'fgtauth\?([a-f0-9]+)') { return $Matches[1] }

    # 2. Try HTML body of the intercept page (if Fortinet returns 200 OK without location headers)
    Log "Debug: No redirect header found, checking HTML body..."
    $body = curl.exe -sk --max-time 10 $CHECK_URL
    if ($body -match 'magic=([a-f0-9]+)') { return $Matches[1] }
    if ($body -match 'fgtauth\?([a-f0-9]+)') { return $Matches[1] }

    # 3. Fallback: Hit the portal directly and scrape the hidden input value
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

    # Step 1: Initialize session
    Log "Step 1: Initializing session via fgtauth..."
    curl.exe -c "$CookieFile" -b "$CookieFile" -skL "${PORTAL}/fgtauth?${magic}" -o NUL

    # Step 2: Submit credentials
    Log "Step 2: Submitting credentials for user $global:USERNAME..."
    $postResponse = curl.exe -c "$CookieFile" -b "$CookieFile" -sk -X POST "${PORTAL}/" `
        --data-urlencode "username=$global:USERNAME" `
        --data-urlencode "password=$global:PASSWORD" `
        --data "magic=$magic" `
        --data "4Tredir=http://connectivitycheck.gstatic.com/generate_204"

    # Step 3: Check for Keepalive
    if ($postResponse -match 'keepalive\?([a-f0-9]+)') {
        $keepalive = $Matches[1]
        Log "SUCCESS: Credentials accepted. Keepalive token: $keepalive"
        
        Log "Step 3: Activating connection..."
        curl.exe -c "$CookieFile" -b "$CookieFile" -skL "${PORTAL}/keepalive?${keepalive}" -o NUL
    } else {
        Log "FAIL: No keepalive found in response. Dumping response to temp file..."
        $postResponse | Out-File (Join-Path $env:TEMP "fortinet_debug.html")
        Log "Check $(Join-Path $env:TEMP 'fortinet_debug.html') to see the error page."
    }

    Start-Sleep -Seconds 2
    return Is-LoggedIn
}

# ── Main ──────────────────────────────────────────────────────────────────────

Log "Starting BITS WiFi Debugger..."
if (Is-LoggedIn) {
    Log "Already authenticated."
    exit 0
}

for ($i = 1; $i -le 2; $i++) {
    Log "--- Attempt $i ---"
    if (Login) {
        Log "[DONE] Login successful!"
        exit 0
    }
    Start-Sleep -Seconds 2
}

Log "[FATAL] All attempts failed. Check the debug logs above."
exit 1