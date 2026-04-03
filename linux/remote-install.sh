#!/usr/bin/env bash
# BITS WiFi Login - Remote Installer (Linux)
# Usage: curl -fsSL https://plasmaDestroyer.github.io/bits-wifi-login/linux/remote-install.sh | bash

set -e

INSTALL_DIR="$HOME/.local/share/bits-wifi-login"
TAR_URL="https://github.com/plasmaDestroyer/bits-wifi-login/archive/refs/heads/main.tar.gz"

echo ""
echo "=== BITS WiFi Auto-Login Installer (Linux) ==="
echo ""

# ── Download & Extract ────────────────────────────────────────────────────────
echo "[1/2] Downloading & Extracting..."
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

curl -fsSL "$TAR_URL" | tar -xz -C "$TMP_DIR"

mkdir -p "$(dirname "$INSTALL_DIR")"
rm -rf "$INSTALL_DIR"
mv "$TMP_DIR/bits-wifi-login-main" "$INSTALL_DIR"

# ── Run installer ─────────────────────────────────────────────────────────────
echo "[2/2] Setting up..."
echo ""
chmod +x "$INSTALL_DIR/linux/install.sh"
exec "$INSTALL_DIR/linux/install.sh"
