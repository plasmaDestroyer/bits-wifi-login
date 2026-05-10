$ErrorActionPreference = "Stop"

function Log {
    param([string]$Message)
    Write-Host ("[{0}] {1}" -f (Get-Date -Format 'HH:mm:ss'), $Message)
}

$MainTaskName = "BITS-WiFi-Login"
$EventTaskName = "BITS-WiFi-Login-OnConnect"

# ── Admin Check ──────────────────────────────────────────────────────────────
$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Error "ERROR: This script must be run as Administrator. Please right-click PowerShell and 'Run as Administrator'."
    exit 1
}

$removed = 0
$warned = 0

function Remove-Task {
    param([string]$TaskName)

    if (Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue) {
        Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
        Log "[OK] Removed scheduled task $TaskName"
        $script:removed++
    } else {
        Log "[WARN] Not found: scheduled task $TaskName"
        $script:warned++
    }
}

# ── Remove scheduled tasks ───────────────────────────────────────────────────

Remove-Task -TaskName $MainTaskName
Remove-Task -TaskName $EventTaskName

# ── Done ─────────────────────────────────────────────────────────────────────

Write-Host "`n[DONE] Uninstall complete."
Write-Host "  Removed/disabled $removed items. ($warned skipped/not found)"
Write-Host "`n  Note: creds.conf was left intact so you don't need to re-enter"
Write-Host "  credentials if you reinstall. Delete it manually if you no longer need it."
