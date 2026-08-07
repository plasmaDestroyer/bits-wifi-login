#!/usr/bin/env bash
# BITS WiFi Login - Remote Installer (macOS)
# Usage: curl -fsSL https://plasmaDestroyer.github.io/bits-wifi-login/mac/remote-install.sh | bash

set -e

INSTALL_DIR="$HOME/.local/share/bits-wifi-login"
REPO="https://github.com/plasmaDestroyer/bits-wifi-login"
TAR_URL="${REPO}/archive/refs/heads/main.tar.gz"

case "$(uname -m)" in
x86_64) ARCH=amd64 ;;
arm64) ARCH=arm64 ;;
*)
    echo "ERROR: no prebuilt binary for $(uname -m). Build from source: go build -o bits-wifi-login ./cmd/bits-wifi-login"
    exit 1
    ;;
esac
# Asset name is set by .github/workflows/release.yml — keep the two in step.
BIN_URL="${REPO}/releases/latest/download/bits-wifi-login-darwin-${ARCH}"

echo ""
echo "=== BITS WiFi Auto-Login Installer (macOS) ==="
echo ""

# ── Download & Extract ────────────────────────────────────────────────────────
echo "[1/3] Downloading & Extracting..."
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

curl -fsSL "$TAR_URL" | tar -xz -C "$TMP_DIR"

mkdir -p "$(dirname "$INSTALL_DIR")"
rm -rf "$INSTALL_DIR"
mv "$TMP_DIR/bits-wifi-login-main" "$INSTALL_DIR"

# ── Fetch the binary ──────────────────────────────────────────────────────────
echo "[2/3] Fetching binary (darwin/${ARCH})..."
curl -fsSL -o "$INSTALL_DIR/bits-wifi-login" "$BIN_URL"
chmod +x "$INSTALL_DIR/bits-wifi-login"

# ── Run installer ─────────────────────────────────────────────────────────────
echo "[3/3] Setting up..."
echo ""
chmod +x "$INSTALL_DIR/mac/install.sh"
exec "$INSTALL_DIR/mac/install.sh"
