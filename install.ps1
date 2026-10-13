# BITS WiFi Login - bootstrap installer (Windows)
# Usage: irm https://plasmaDestroyer.github.io/bits-wifi-login/install.ps1 | iex
#
# This only fetches the binary. Everything else - credentials, background
# triggers, uninstall - lives in `bits-wifi-login install`.

[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingWriteHost', '')]
param()

$ErrorActionPreference = "Stop"

$Repo = "https://github.com/plasmaDestroyer/bits-wifi-login"
$InstallDir = "$env:LOCALAPPDATA\bits-wifi-login"
$Bin = "$InstallDir\bits-wifi-login.exe"
# Asset name is set by .github/workflows/release.yml - keep the two in step.
$BinUrl = "$Repo/releases/latest/download/bits-wifi-login-windows-amd64.exe"

Write-Host ""
Write-Host "=== BITS WiFi Auto-Login Installer (windows/amd64) ===" -ForegroundColor Cyan
Write-Host ""

Write-Host "[1/2] Downloading..." -ForegroundColor Cyan
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
# Windows will not overwrite a running .exe but will rename one, and a watcher
# may be running it right now. The .old copy is cleared on the next run.
Invoke-WebRequest -Uri $BinUrl -OutFile "$Bin.new"
Unblock-File -Path "$Bin.new"
Remove-Item "$Bin.old" -Force -ErrorAction SilentlyContinue
if (Test-Path $Bin) { Move-Item $Bin "$Bin.old" -Force }
Move-Item "$Bin.new" $Bin -Force

Write-Host "[2/2] Setting up..." -ForegroundColor Cyan
Write-Host ""
& $Bin install
