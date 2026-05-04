# ── BITS WiFi Login - Remote Installer ────────────────────────────────────────
# Usage: irm https://plasmaDestroyer.github.io/bits-wifi-login/windows/remote-install.ps1 | iex

$ErrorActionPreference = "Stop"

# ── Admin Check ───────────────────────────────────────────────────────────────
$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Host "`n[ERROR] Please run this in an Administrator PowerShell." -ForegroundColor Red
    Write-Host "        Right-click PowerShell -> 'Run as Administrator'`n" -ForegroundColor Yellow
    return
}

$InstallDir = "$env:LOCALAPPDATA\bits-wifi-login"
$ZipUrl = "https://github.com/plasmaDestroyer/bits-wifi-login/archive/refs/heads/main.zip"
$ZipFile = "$env:TEMP\bits-wifi-login.zip"
$ExtractedDir = "$env:TEMP\bits-wifi-login-main"

Write-Host ""
Write-Host "=== BITS WiFi Auto-Login Installer ===" -ForegroundColor Cyan
Write-Host ""

try {
    # ── Download ──────────────────────────────────────────────────────────────
    Write-Host "[1/3] Downloading..." -ForegroundColor Cyan
    Invoke-WebRequest -Uri $ZipUrl -OutFile $ZipFile

    # ── Extract ───────────────────────────────────────────────────────────────
    Write-Host "[2/3] Extracting..." -ForegroundColor Cyan

    if (Test-Path $ExtractedDir) { Remove-Item $ExtractedDir -Recurse -Force }
    if (Test-Path $InstallDir) { Remove-Item $InstallDir -Recurse -Force }

    Expand-Archive -Path $ZipFile -DestinationPath $env:TEMP -Force
    Move-Item $ExtractedDir $InstallDir

    # ── Run installer ─────────────────────────────────────────────────────────
    Write-Host "[3/3] Setting up..." -ForegroundColor Cyan
    Write-Host ""

    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$InstallDir\windows\install.ps1"
} finally {
    Remove-Item $ZipFile -Force -ErrorAction SilentlyContinue
    Remove-Item $ExtractedDir -Recurse -Force -ErrorAction SilentlyContinue
}
