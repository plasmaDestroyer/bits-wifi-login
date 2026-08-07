[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingWriteHost', '')]
param()

$ErrorActionPreference = "Stop"

function Log {
    param([string]$Message)
    Write-Host ("[{0}] {1}" -f (Get-Date -Format 'HH:mm:ss'), $Message)
}

function Show-FailHint {
    param([string]$Message)

    Log "ERROR: install failed: $Message"
    Log "Some files or scheduled tasks may have been installed already. Re-run after fixing the error, or uninstall manually."
}

function ConvertTo-EscapedCredsValue {
    param([string]$Value)

    $escaped = $Value.Replace('\', '\\')
    $escaped = $escaped.Replace('"', '\"')
    $escaped = $escaped.Replace("`r", '\r')
    $escaped = $escaped.Replace("`n", '\n')
    $escaped = $escaped.Replace("`t", '\t')

    return '"' + $escaped + '"'
}

function ConvertTo-EscapedXmlText {
    param([string]$Value)

    return [System.Security.SecurityElement]::Escape($Value)
}

function Write-CredsFile {
    param(
        [string]$Path,
        [PSCredential]$Credential
    )

    $user = $Credential.UserName
    # creds.conf is intentionally plain text — PSAvoidUsingPlainTextForPassword does not apply here
    $plainText = $Credential.GetNetworkCredential().Password
    @(
        "USERNAME=$(ConvertTo-EscapedCredsValue $user)"
        "PASSWORD=$(ConvertTo-EscapedCredsValue $plainText)"
    ) | Set-Content -Path $Path -Encoding UTF8
}

# The binary logs to stdout; a scheduled task discards that, so route it through
# cmd.exe to keep a log file (Windows has no journalctl equivalent here).
function Get-TaskCommandLine {
    param(
        [string]$BinaryPath,
        [string]$LogFile
    )

    return "/c `"`"$BinaryPath`" >> `"$LogFile`" 2>&1`""
}

function Get-TaskXml {
    param(
        [string]$TaskUser,
        [string]$BinaryPath,
        [string]$LogFile,
        [string]$PeriodicStartBoundary
    )

    $escapedTaskUser = ConvertTo-EscapedXmlText $TaskUser
    $escapedTaskArgs = ConvertTo-EscapedXmlText (Get-TaskCommandLine -BinaryPath $BinaryPath -LogFile $LogFile)
    $escapedStartBoundary = ConvertTo-EscapedXmlText $PeriodicStartBoundary

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
      <Command>cmd.exe</Command>
      <Arguments>$escapedTaskArgs</Arguments>
    </Exec>
  </Actions>
</Task>
"@
}

function Get-EventTaskXml {
    param(
        [string]$TaskUser,
        [string]$BinaryPath,
        [string]$LogFile
    )

    $escapedTaskUser = ConvertTo-EscapedXmlText $TaskUser
    $escapedTaskArgs = ConvertTo-EscapedXmlText (Get-TaskCommandLine -BinaryPath $BinaryPath -LogFile $LogFile)

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
      <Command>cmd.exe</Command>
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
Write-Host "[INFO] Registering scheduled tasks for user: $TaskUser — if this looks wrong (e.g. an admin account instead of your login), re-run the installer as the correct user." -ForegroundColor Yellow
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoDir = Split-Path -Parent $ScriptDir
$BinaryPath = Join-Path $RepoDir "bits-wifi-login.exe"
$CredsFile = Join-Path $RepoDir "creds.conf"
$LogFile = Join-Path $RepoDir "bits-wifi-login.log"
$MainTaskName = "BITS-WiFi-Login"
$EventTaskName = "BITS-WiFi-Login-OnConnect"

# ── Preflight checks ──────────────────────────────────────────────────────────

if (-not (Test-Path $BinaryPath)) {
    Log "ERROR: bits-wifi-login.exe not found at $BinaryPath"
    Log "  Download it from the latest release, or build it: go build -o bits-wifi-login.exe ./cmd/bits-wifi-login"
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
    $cred = New-Object PSCredential($inputUser, $inputPass)
    try {
        Write-CredsFile -Path $CredsFile -Credential $cred
    } finally {
        # PSCredential handles secure memory cleanup
    }
    Log "[OK] creds.conf created."
} else {
    Log "[OK] creds.conf already exists, skipping."
}

# ── Register scheduled tasks ─────────────────────────────────────────────────

$periodicStartBoundary = (Get-Date).AddMinutes(1).ToString("s")
$mainTaskXml = Get-TaskXml -TaskUser $TaskUser -BinaryPath $BinaryPath -LogFile $LogFile -PeriodicStartBoundary $periodicStartBoundary
$eventTaskXml = Get-EventTaskXml -TaskUser $TaskUser -BinaryPath $BinaryPath -LogFile $LogFile

$mainTaskXmlPath = Join-Path $env:TEMP "bits-wifi-main.xml"
$eventTaskXmlPath = Join-Path $env:TEMP "bits-wifi-connect.xml"

try {
    $mainTaskXml | Out-File $mainTaskXmlPath -Encoding Unicode
    $eventTaskXml | Out-File $eventTaskXmlPath -Encoding Unicode

    Register-ScheduledTask `
        -TaskName $MainTaskName `
        -Xml (Get-Content $mainTaskXmlPath -Raw) `
        -Force | Out-Null
    if (-not (Get-ScheduledTask -TaskName $MainTaskName -ErrorAction SilentlyContinue)) {
        throw "Main scheduled task was not registered."
    }
    Log "[OK] Main scheduled task registered (every 30 minutes and on login)."

    Register-ScheduledTask `
        -TaskName $EventTaskName `
        -Xml (Get-Content $eventTaskXmlPath -Raw) `
        -Force | Out-Null
    if (-not (Get-ScheduledTask -TaskName $EventTaskName -ErrorAction SilentlyContinue)) {
        throw "Network/resume scheduled task was not registered."
    }
    Log "[OK] Network and resume trigger task registered."
} catch {
    Show-FailHint $_.Exception.Message
    exit 1
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
Write-Host "`n  Repair:"
Write-Host "    Re-run this installer if files, tasks, permissions, or background triggers break."
