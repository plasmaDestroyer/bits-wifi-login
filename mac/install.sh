#!/usr/bin/env bash

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT_PATH="${SCRIPT_DIR}/fortinet-login.sh"
PLIST=~/Library/LaunchAgents/ac.bits.wifi-login.plist

log() { echo "[$(date '+%H:%M:%S')] $*"; }

# ── Preflight checks ──────────────────────────────────────────────────────────

if [[ ! -f "$SCRIPT_PATH" ]]; then
    log "ERROR: fortinet-login.sh not found at $SCRIPT_PATH"
    exit 1
fi

if ! command -v curl &>/dev/null; then
    log "ERROR: curl not found."
    exit 1
fi

# ── Credentials ───────────────────────────────────────────────────────────────

if [[ ! -f "${SCRIPT_DIR}/creds.conf" ]]; then
    log "No creds.conf found. Let's create one."
    read -rp "Enter your BITS username: " input_user
    read -rsp "Enter your BITS password: " input_pass
    echo ""
    cat > "${SCRIPT_DIR}/creds.conf" << EOF
USERNAME="${input_user}"
PASSWORD="${input_pass}"
EOF
    chmod 600 "${SCRIPT_DIR}/creds.conf"
    log "✓ creds.conf created."
else
    log "✓ creds.conf already exists, skipping."
fi

# ── Make script executable ────────────────────────────────────────────────────

chmod +x "$SCRIPT_PATH"
log "✓ Script permissions set."

# ── launchd plist ─────────────────────────────────────────────────────────────

mkdir -p ~/Library/LaunchAgents

cat > "$PLIST" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>ac.bits.wifi-login</string>

    <key>ProgramArguments</key>
    <array>
        <string>/bin/bash</string>
        <string>${SCRIPT_PATH}</string>
    </array>

    <key>WatchPaths</key>
    <array>
        <string>/var/run/resolv.conf</string>
    </array>

    <key>StartInterval</key>
    <integer>1800</integer>

    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>
EOF
log "✓ launchd plist created."

# ── Load the agent ────────────────────────────────────────────────────────────

launchctl unload "$PLIST" 2>/dev/null || true
launchctl load "$PLIST"
log "✓ launchd agent loaded."

# ── Done ──────────────────────────────────────────────────────────────────────

echo ""
echo "✓ Installation complete."
echo ""
echo "  Triggers:"
echo "    - Every WiFi connect to BITS-STUDENT (resolv.conf watch)"
echo "    - Every 30 minutes (StartInterval)"
echo "    - On login (RunAtLoad)"
echo ""
echo "  Logs:"
echo "    log show --predicate 'processImagePath contains \"bash\"' --info --last 1h | grep fortinet"
echo ""
echo "  Uninstall:"
echo "    launchctl unload ~/Library/LaunchAgents/ac.bits.wifi-login.plist"
echo "    rm ~/Library/LaunchAgents/ac.bits.wifi-login.plist"
