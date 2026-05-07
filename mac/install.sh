#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT_PATH="${SCRIPT_DIR}/fortinet-login.sh"
LABEL="ac.bits.wifi-login"
PLIST="$HOME/Library/LaunchAgents/${LABEL}.plist"
LAUNCHD_DOMAIN="gui/$(id -u)"
LAUNCHD_SERVICE="${LAUNCHD_DOMAIN}/${LABEL}"

log() { echo "[$(date '+%H:%M:%S')] $*"; }

fail_hint() {
    log "ERROR: install failed at line $1."
    log "Some files may have been installed already. Re-run after fixing the error, or uninstall manually."
}

trap 'fail_hint "$LINENO"' ERR

# ── Preflight checks ──────────────────────────────────────────────────────────

if [[ "$(id -u)" == "0" ]]; then
    log "ERROR: Do not run the macOS installer with sudo. It installs a per-user LaunchAgent."
    exit 1
fi

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
    read -rp "Enter your BITS username: " input_user </dev/tty
    read -rsp "Enter your BITS password: " input_pass </dev/tty
    echo ""
    {
        printf "USERNAME='%s'\n" "${input_user//\'/\'\\\'\'}"
        printf "PASSWORD='%s'\n" "${input_pass//\'/\'\\\'\'}"
    } > "${SCRIPT_DIR}/creds.conf"
    chmod 600 "${SCRIPT_DIR}/creds.conf"
    [[ -f "${SCRIPT_DIR}/creds.conf" ]]
    log "✓ creds.conf created."
else
    log "✓ creds.conf already exists, skipping."
fi

# ── Make script executable ────────────────────────────────────────────────────

chmod +x "$SCRIPT_PATH"
[[ -x "$SCRIPT_PATH" ]]
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
    <string>${LABEL}</string>

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
plutil -lint "$PLIST" >/dev/null
[[ -f "$PLIST" ]]
log "✓ launchd plist created."

# ── Load the agent ────────────────────────────────────────────────────────────

launchctl bootout "$LAUNCHD_SERVICE" 2>/dev/null || true
launchctl bootstrap "$LAUNCHD_DOMAIN" "$PLIST"
launchctl enable "$LAUNCHD_SERVICE" 2>/dev/null || true
launchctl print "$LAUNCHD_SERVICE" >/dev/null
log "✓ launchd agent loaded."

# ── Done ──────────────────────────────────────────────────────────────────────

echo ""
echo "✓ Installation complete."
echo ""
echo "  Triggers:"
echo "    - Every WiFi connect to BITS-STUDENT (resolv.conf watch)"
echo "    - Every 30 minutes (StartInterval - launchd makes this persistent across sleep)"
echo "    - Every resume from sleep (macOS launchd immediately fires missed StartIntervals on wake)"
echo "    - On login (RunAtLoad)"
echo ""
echo "  Logs:"
echo "    log show --predicate 'processImagePath contains \"bash\"' --info --last 1h | grep fortinet"
echo ""
echo "  Uninstall:"
echo "    launchctl unload ~/Library/LaunchAgents/ac.bits.wifi-login.plist"
echo "    rm ~/Library/LaunchAgents/ac.bits.wifi-login.plist"
