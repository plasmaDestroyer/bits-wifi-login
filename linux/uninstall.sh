#!/usr/bin/env bash

set -euo pipefail

log() { echo "[$(date '+%H:%M:%S')] $*"; }

removed=0
warned=0

remove_file() {
    if [[ -f "$1" ]]; then
        sudo rm -f "$1"
        log "✓ Removed $1"
        ((removed++)) || true
    else
        log "⚠ Not found: $1"
        ((warned++)) || true
    fi
}

disable_unit() {
    local unit="$1"
    # systemctl disable returns 0 if successful (even if already disabled)
    # and >0 if the unit does not exist or failed to disable
    if sudo systemctl disable --now "$unit" >/dev/null 2>&1; then
        log "✓ Disabled and stopped $unit"
        ((removed++)) || true
    else
        log "⚠ Not found or already removed: $unit"
        ((warned++)) || true
    fi
}

# 1. Stop & disable units unconditionally to ensure symlinks are removed
disable_unit "bits-wifi-login.timer"
disable_unit "bits-wifi-login.service"
disable_unit "bits-wifi-login-resume.service"

# 2. Remove unit files
remove_file "/etc/systemd/system/bits-wifi-login.timer"
remove_file "/etc/systemd/system/bits-wifi-login.service"
remove_file "/etc/systemd/system/bits-wifi-login-resume.service"

# 3. systemctl daemon-reload
sudo systemctl daemon-reload
log "✓ Reloaded systemd daemon"

# 4. Remove NM dispatcher script
remove_file "/etc/NetworkManager/dispatcher.d/90-fortinet-login"

echo ""
echo "✓ Uninstall complete."
echo "  Removed/disabled $removed items. ($warned skipped/not found)"
echo ""
echo "  Note: creds.conf was left intact so you don't need to re-enter"
echo "  credentials if you reinstall. Delete it manually if you no longer need it."
