#!/usr/bin/env bash

set -euo pipefail

log() { echo "[$(date '+%H:%M:%S')] $*"; }

if [[ "$EUID" -ne 0 ]]; then
    log "ERROR: Please run this script with sudo."
    exit 1
fi

removed=0
warned=0

remove_file() {
    if [[ -f "$1" ]]; then
        rm -f "$1"
        log "✓ Removed $1"
        ((removed++))
    else
        log "⚠ Not found: $1"
        ((warned++))
    fi
}

# 1. Stop & disable timer
if systemctl is-active --quiet bits-wifi-login.timer 2>/dev/null; then
    systemctl disable --now bits-wifi-login.timer >/dev/null 2>&1 || true
    log "✓ Disabled timer bits-wifi-login.timer"
    ((removed++))
else
    log "⚠ Not active: bits-wifi-login.timer"
    ((warned++))
fi

# 2. Disable resume service
if systemctl is-enabled --quiet bits-wifi-login-resume.service 2>/dev/null; then
    systemctl disable bits-wifi-login-resume.service >/dev/null 2>&1 || true
    log "✓ Disabled service bits-wifi-login-resume.service"
    ((removed++))
else
    log "⚠ Not enabled: bits-wifi-login-resume.service"
    ((warned++))
fi

# 3. Remove unit files
remove_file "/etc/systemd/system/bits-wifi-login.timer"
remove_file "/etc/systemd/system/bits-wifi-login.service"
remove_file "/etc/systemd/system/bits-wifi-login-resume.service"

# 4. systemctl daemon-reload
systemctl daemon-reload
log "✓ Reloaded systemd daemon"

# 5. Remove NM dispatcher script
remove_file "/etc/NetworkManager/dispatcher.d/90-fortinet-login"

echo ""
echo "✓ Uninstall complete."
echo "  Removed/disabled $removed items. ($warned skipped/not found)"
echo ""
echo "  Note: creds.conf was left intact so you don't need to re-enter"
echo "  credentials if you reinstall. Delete it manually if you no longer need it."
