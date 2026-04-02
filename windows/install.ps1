$ErrorActionPreference = "Stop"

# ── Admin Check ──────────────────────────────────────────────────────────────
$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Error "ERROR: This script must be run as Administrator. Please right-click PowerShell and 'Run as Administrator'."
    exit 1
}

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoDir = Split-Path -Parent $ScriptDir
$LoginScript = Join-Path $ScriptDir "fortinet-login.ps1"
$CredsFile = Join-Path $RepoDir "creds.conf"

function Log { param($msg) Write-Host "[$(Get-Date -Format 'HH:mm:ss')] $msg" }

# ── Preflight checks ──────────────────────────────────────────────────────────

if (-not (Test-Path $LoginScript)) {
    Log "ERROR: fortinet-login.ps1 not found at $LoginScript"
    exit 1
}

if (-not (Get-Command curl.exe -ErrorAction SilentlyContinue)) {
    Log "ERROR: curl not found. Please update Windows 10 or install curl."
    exit 1
}

# ── Credentials ───────────────────────────────────────────────────────────────

if (-not (Test-Path $CredsFile)) {
    Log "No creds.conf found. Let's create one."
    $inputUser = Read-Host "Enter your BITS username"
    $inputPass = Read-Host "Enter your BITS password" -AsSecureString
    $plainPass = [Runtime.InteropServices.Marshal]::PtrToStringAuto(
        [Runtime.InteropServices.Marshal]::SecureStringToBSTR($inputPass)
    )
@"
USERNAME="$inputUser"
PASSWORD="$plainPass"
"@ | Set-Content $CredsFile
    Log "[OK] creds.conf created."
} else {
    Log "[OK] creds.conf already exists, skipping."
}

# ── Register scheduled tasks ──────────────────────────────────────────────────

$action = New-ScheduledTaskAction `
    -Execute "powershell.exe" `
    -Argument "-WindowStyle Hidden -ExecutionPolicy Bypass -File `"$LoginScript`""

$bootTrigger = New-ScheduledTaskTrigger -AtStartup
$bootTrigger.Repetition.Interval = "PT30M"
$loginTrigger = New-ScheduledTaskTrigger -AtLogOn

$settings = New-ScheduledTaskSettingsSet `
    -RunOnlyIfNetworkAvailable `
    -ExecutionTimeLimit (New-TimeSpan -Minutes 2) `
    -StartWhenAvailable `
    -Hidden

$principal = New-ScheduledTaskPrincipal `
    -UserId $env:USERNAME `
    -LogonType Interactive `
    -RunLevel Limited

# Register boot + login triggers
Register-ScheduledTask `
    -TaskName "BITS-WiFi-Login" `
    -Action $action `
    -Trigger $bootTrigger, $loginTrigger `
    -Settings $settings `
    -Principal $principal `
    -Force | Out-Null
Log "[OK] Scheduled task registered (boot and login triggers)."

# ── Register network event trigger via XML ────────────────────────────────────

$fullTaskXml = @"
<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Triggers>
    <EventTrigger>
      <Subscription>&lt;QueryList&gt;&lt;Query Id="0" Path="Microsoft-Windows-NetworkProfile/Operational"&gt;&lt;Select Path="Microsoft-Windows-NetworkProfile/Operational"&gt;*[System[EventID=10000]]&lt;/Select&gt;&lt;/Query&gt;&lt;/QueryList&gt;</Subscription>
      <Delay>PT3S</Delay>
    </EventTrigger>
    <EventTrigger>
      <Subscription>&lt;QueryList&gt;&lt;Query Id="0" Path="System"&gt;&lt;Select Path="System"&gt;*[System[Provider[@Name='Microsoft-Windows-Power-Troubleshooter'] and EventID=1]]&lt;/Select&gt;&lt;/Query&gt;&lt;/QueryList&gt;</Subscription>
      <Delay>PT5S</Delay>
    </EventTrigger>
  </Triggers>
  <Actions>
    <Exec>
      <Command>powershell.exe</Command>
      <Arguments>-WindowStyle Hidden -ExecutionPolicy Bypass -File \"$LoginScript\"</Arguments>
    </Exec>
  </Actions>
  <Settings>
    <Hidden>true</Hidden>
    <RunOnlyIfNetworkAvailable>true</RunOnlyIfNetworkAvailable>
    <ExecutionTimeLimit>PT2M</ExecutionTimeLimit>
  </Settings>
  <Principals>
    <Principal>
      <UserId>$env:USERNAME</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
</Task>
"@

$tempXmlPath = Join-Path $env:TEMP "bits-wifi-connect.xml"
$fullTaskXml | Out-File $tempXmlPath -Encoding Unicode

Register-ScheduledTask `
    -TaskName "BITS-WiFi-Login-OnConnect" `
    -Xml (Get-Content $tempXmlPath -Raw) `
    -Force | Out-Null
Log "[OK] Network connect trigger registered."

# ── Done ──────────────────────────────────────────────────────────────────────

Write-Host "`n[DONE] Installation complete.`n"
Write-Host "  Triggers:"
Write-Host "    - Every WiFi connect (Event ID 10000)"
Write-Host "    - Every resume from sleep (Power-Troubleshooter Event ID 1)"
Write-Host "    - Every 30 minutes (Interval repeating task)"
Write-Host "    - On boot and login"
Write-Host "`n  Logs:"
Write-Host "    Get-EventLog -LogName Application -Source 'BITS-WiFi-Login' -Newest 20"
Write-Host "`n  Uninstall:"
Write-Host "    Unregister-ScheduledTask -TaskName 'BITS-WiFi-Login' -Confirm:`$false"
Write-Host "    Unregister-ScheduledTask -TaskName 'BITS-WiFi-Login-OnConnect' -Confirm:`$false"