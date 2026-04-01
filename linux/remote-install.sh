#!/usr/bin/env bash
# BITS WiFi Login - Remote Installer (Linux)
# Usage: curl -fsSL https://raw.githubusercontent.com/plasmaDestroyer/bits-wifi-login/main/linux/remote-install.sh | bash

set -e

INSTALL_DIR="$HOME/.local/share/bits-wifi-login"
ZIP_URL="https://github.com/plasmaDestroyer/bits-wifi-login/archive/refs/heads/main.zip"
TMP_ZIP="/tmp/bits-wifi-login.zip"

echo ""
echo "=== BITS WiFi Auto-Login Installer (Linux) ==="
echo ""

# ── Download ──────────────────────────────────────────────────────────────────
echo "[1/3] Downloading..."
curl -fsSL "$ZIP_URL" -o "$TMP_ZIP"

# ── Extract ───────────────────────────────────────────────────────────────────
echo "[2/3] Extracting..."
rm -rf "$INSTALL_DIR"
unzip -qo "$TMP_ZIP" -d /tmp
mv /tmp/bits-wifi-login-main "$INSTALL_DIR"
rm -f "$TMP_ZIP"

# ── Run installer ─────────────────────────────────────────────────────────────
echo "[3/3] Setting up..."
echo ""
chmod +x "$INSTALL_DIR/linux/install.sh"
exec "$INSTALL_DIR/linux/install.sh"
