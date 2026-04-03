$ErrorActionPreference = "Stop"

function Log {
    param([string]$Message)
    Write-Host ("[{0}] {1}" -f (Get-Date -Format 'HH:mm:ss'), $Message)
}

function Escape-CredsValue {
    param([string]$Value)

    $escaped = $Value.Replace('\', '\\')
    $escaped = $escaped.Replace('"', '\"')
    $escaped = $escaped.Replace("`r", '\r')
    $escaped = $escaped.Replace("`n", '\n')
    $escaped = $escaped.Replace("`t", '\t')

    return '"' + $escaped + '"'
}

function Escape-XmlText {
    param([string]$Value)

    return [System.Security.SecurityElement]::Escape($Value)
}

function Write-CredsFile {
    param(
        [string]$Path,
        [string]$Username,
        [string]$Password
    )

    @(
        "USERNAME=$(Escape-CredsValue $Username)"
        "PASSWORD=$(Escape-CredsValue $Password)"
    ) | Set-Content -Path $Path -Encoding UTF8
}

function New-TaskXml {
    param(
        [string]$TaskUser,
        [string]$LoginScript,
        [string]$PeriodicStartBoundary
    )

    $escapedTaskUser = Escape-XmlText $TaskUser
    $escapedTaskArgs = Escape-XmlText "-NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -File `"$LoginScript`""
    $escapedStartBoundary = Escape-XmlText $PeriodicStartBoundary

    return @"
<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo />
  <Triggers>
    <TimeTrigger>
      <StartBoundary>$escapedStartBoundary</StartBoundary>
      <Enabled>true</Enabled>
      <Repetition>
        <Interval>PT30M</Interval>
        <Duration>P9999D</Duration>
      </Repetition>
    </TimeTrigger>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>$escapedTaskUser</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>$escapedTaskUser</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>true</RunOnlyIfNetworkAvailable>
    <ExecutionTimeLimit>PT2M</ExecutionTimeLimit>
    <Enabled>true</Enabled>
    <Hidden>true</Hidden>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>powershell.exe</Command>
      <Arguments>$escapedTaskArgs</Arguments>
    </Exec>
  </Actions>
</Task>
"@
}

function New-EventTaskXml {
    param(
        [string]$TaskUser,
        [string]$LoginScript
    )

    $escapedTaskUser = Escape-XmlText $TaskUser
    $escapedTaskArgs = Escape-XmlText "-NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -File `"$LoginScript`""

    return @"
<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo />
  <Triggers>
    <EventTrigger>
      <Enabled>true</Enabled>
      <Subscription>&lt;QueryList&gt;&lt;Query Id="0" Path="Microsoft-Windows-NetworkProfile/Operational"&gt;&lt;Select Path="Microsoft-Windows-NetworkProfile/Operational"&gt;*[System[EventID=10000]]&lt;/Select&gt;&lt;/Query&gt;&lt;/QueryList&gt;</Subscription>
      <Delay>PT3S</Delay>
    </EventTrigger>
    <EventTrigger>
      <Enabled>true</Enabled>
      <Subscription>&lt;QueryList&gt;&lt;Query Id="0" Path="System"&gt;&lt;Select Path="System"&gt;*[System[Provider[@Name='Microsoft-Windows-Power-Troubleshooter'] and EventID=1]]&lt;/Select&gt;&lt;/Query&gt;&lt;/QueryList&gt;</Subscription>
      <Delay>PT5S</Delay>
    </EventTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>$escapedTaskUser</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>true</RunOnlyIfNetworkAvailable>
    <ExecutionTimeLimit>PT2M</ExecutionTimeLimit>
    <Enabled>true</Enabled>
    <Hidden>true</Hidden>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>powershell.exe</Command>
      <Arguments>$escapedTaskArgs</Arguments>
    </Exec>
  </Actions>
</Task>
"@
}

# ── Admin Check ──────────────────────────────────────────────────────────────
$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Error "ERROR: This script must be run as Administrator. Please right-click PowerShell and 'Run as Administrator'."
    exit 1
}

$CurrentIdentity = [Security.Principal.WindowsIdentity]::GetCurrent()
$TaskUser = $CurrentIdentity.Name
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoDir = Split-Path -Parent $ScriptDir
$LoginScript = Join-Path $ScriptDir "fortinet-login.ps1"
$CredsFile = Join-Path $RepoDir "creds.conf"
$LogFile = Join-Path $RepoDir "fortinet-login.log"
$MainTaskName = "BITS-WiFi-Login"
$EventTaskName = "BITS-WiFi-Login-OnConnect"

# ── Preflight checks ──────────────────────────────────────────────────────────

if (-not (Test-Path $LoginScript)) {
    Log "ERROR: fortinet-login.ps1 not found at $LoginScript"
    exit 1
}

if (-not (Get-Command curl.exe -ErrorAction SilentlyContinue)) {
    Log "ERROR: curl not found. Please update Windows 10 or install curl."
    exit 1
}

if (-not (Get-Command Register-ScheduledTask -ErrorAction SilentlyContinue)) {
    Log "ERROR: ScheduledTasks module is unavailable on this system."
    exit 1
}

# ── Credentials ───────────────────────────────────────────────────────────────

if (-not (Test-Path $CredsFile)) {
    Log "No creds.conf found. Let's create one."
    $inputUser = Read-Host "Enter your BITS username"
    $inputPass = Read-Host "Enter your BITS password" -AsSecureString
    $bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($inputPass)
    try {
        $plainPass = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)
        Write-CredsFile -Path $CredsFile -Username $inputUser -Password $plainPass
    } finally {
        if ($bstr -ne [IntPtr]::Zero) {
            [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
        }
    }
    Log "[OK] creds.conf created."
} else {
    Log "[OK] creds.conf already exists, skipping."
}

# ── Register scheduled tasks ─────────────────────────────────────────────────

$periodicStartBoundary = (Get-Date).AddMinutes(1).ToString("s")
$mainTaskXml = New-TaskXml -TaskUser $TaskUser -LoginScript $LoginScript -PeriodicStartBoundary $periodicStartBoundary
$eventTaskXml = New-EventTaskXml -TaskUser $TaskUser -LoginScript $LoginScript

$mainTaskXmlPath = Join-Path $env:TEMP "bits-wifi-main.xml"
$eventTaskXmlPath = Join-Path $env:TEMP "bits-wifi-connect.xml"

try {
    $mainTaskXml | Out-File $mainTaskXmlPath -Encoding Unicode
    $eventTaskXml | Out-File $eventTaskXmlPath -Encoding Unicode

    Register-ScheduledTask `
        -TaskName $MainTaskName `
        -Xml (Get-Content $mainTaskXmlPath -Raw) `
        -Force | Out-Null
    Log "[OK] Main scheduled task registered (every 30 minutes and on login)."

    Register-ScheduledTask `
        -TaskName $EventTaskName `
        -Xml (Get-Content $eventTaskXmlPath -Raw) `
        -Force | Out-Null
    Log "[OK] Network and resume trigger task registered."
} finally {
    Remove-Item $mainTaskXmlPath -Force -ErrorAction SilentlyContinue
    Remove-Item $eventTaskXmlPath -Force -ErrorAction SilentlyContinue
}

# ── Done ──────────────────────────────────────────────────────────────────────

Write-Host "`n[DONE] Installation complete.`n"
Write-Host "  Triggers:"
Write-Host "    - Every WiFi connect (NetworkProfile Event ID 10000)"
Write-Host "    - Every resume from sleep (Power-Troubleshooter Event ID 1)"
Write-Host "    - Every 30 minutes"
Write-Host "    - On login"
Write-Host "`n  Logs:"
Write-Host "    Get-Content '$LogFile' -Tail 50"
Write-Host "`n  Uninstall:"
Write-Host "    Unregister-ScheduledTask -TaskName '$MainTaskName' -Confirm:`$false"
Write-Host "    Unregister-ScheduledTask -TaskName '$EventTaskName' -Confirm:`$false"
