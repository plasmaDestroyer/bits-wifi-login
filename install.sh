#!/usr/bin/env bash
# BITS WiFi Login - bootstrap installer (Linux and macOS)
# Usage: curl -fsSL https://plasmaDestroyer.github.io/bits-wifi-login/install.sh | bash
#
# This only fetches the binary. Everything else — credentials, background
# triggers, uninstall — lives in `bits-wifi-login install`.

set -e

REPO="https://github.com/plasmaDestroyer/bits-wifi-login"
INSTALL_DIR="$HOME/.local/share/bits-wifi-login"
BIN="$INSTALL_DIR/bits-wifi-login"

case "$(uname -s)" in
Linux) OS=linux ;;
Darwin) OS=darwin ;;
*)
    echo "ERROR: unsupported OS $(uname -s)."
    exit 1
    ;;
esac

case "$(uname -m)" in
x86_64) ARCH=amd64 ;;
aarch64 | arm64) ARCH=arm64 ;;
*)
    echo "ERROR: no prebuilt binary for $(uname -m). Build from source: go build -o bits-wifi-login ./cmd/bits-wifi-login"
    exit 1
    ;;
esac

# Asset name is set by .github/workflows/release.yml - keep the two in step.
BIN_URL="${REPO}/releases/latest/download/bits-wifi-login-${OS}-${ARCH}"

echo ""
echo "=== BITS WiFi Auto-Login Installer (${OS}/${ARCH}) ==="
echo ""

echo "[1/2] Downloading..."
mkdir -p "$INSTALL_DIR"
# --progress-bar, not -s: this is ~7 MB and silence for the length of a download
# on campus wifi reads as a hang. -f still fails the script on an HTTP error, and
# the bar goes to stderr so it cannot end up inside the file.
curl -f -L --progress-bar -o "$BIN" "$BIN_URL"
chmod +x "$BIN"

echo "[2/2] Setting up..."
echo ""
# `curl | bash` leaves stdin pointing at this script, so the credential prompt
# has to be handed the terminal explicitly.
exec "$BIN" install </dev/tty
