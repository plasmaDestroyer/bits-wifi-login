#!/usr/bin/env bash
# Bootstrap: fetch the binary, let `bits-wifi-login install` do the rest.

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
# Progress bar: ~7 MB of silence on campus wifi reads as a hang.
# Side file + rename: a watcher may be running the old binary (ETXTBSY).
curl -f -L --progress-bar -o "$BIN.new" "$BIN_URL"
chmod +x "$BIN.new"
mv -f "$BIN.new" "$BIN"

echo "[2/2] Setting up..."
echo ""
# `curl | bash` owns stdin, so hand the prompt the terminal.
exec "$BIN" install </dev/tty
