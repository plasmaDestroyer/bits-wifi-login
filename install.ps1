# Bootstrap: fetch the binary, let `bits-wifi-login install` do the rest.

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
# Windows can rename a running .exe but not overwrite it. .old goes next run.
Invoke-WebRequest -Uri $BinUrl -OutFile "$Bin.new"
Unblock-File -Path "$Bin.new"
Remove-Item "$Bin.old" -Force -ErrorAction SilentlyContinue
if (Test-Path $Bin) { Move-Item $Bin "$Bin.old" -Force }
Move-Item "$Bin.new" $Bin -Force

Write-Host "[2/2] Setting up..." -ForegroundColor Cyan
Write-Host ""
& $Bin install
