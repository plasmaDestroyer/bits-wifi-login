#!/usr/bin/env bash

set -euo pipefail

LABEL="ac.bits.wifi-login"
PLIST="$HOME/Library/LaunchAgents/${LABEL}.plist"
LAUNCHD_SERVICE="gui/$(id -u)/${LABEL}"

log() { echo "[$(date '+%H:%M:%S')] $*"; }

if [[ "$(id -u)" == "0" ]]; then
    log "ERROR: Do not run the macOS uninstaller with sudo. It removes a per-user LaunchAgent."
    exit 1
fi

removed=0
warned=0

# 1. Bootout the agent
if launchctl bootout "$LAUNCHD_SERVICE" >/dev/null 2>&1; then
    log "✓ Unloaded launchd agent"
    ((removed++)) || true
else
    log "⚠ Not loaded or already unloaded: launchd agent"
    ((warned++)) || true
fi

# 2. Remove plist
if [[ -f "$PLIST" ]]; then
    rm -f "$PLIST"
    log "✓ Removed $PLIST"
    ((removed++)) || true
else
    log "⚠ Not found: $PLIST"
    ((warned++)) || true
fi

echo ""
echo "✓ Uninstall complete."
echo "  Removed/disabled $removed items. ($warned skipped/not found)"
echo ""
echo "  Note: creds.conf was left intact so you don't need to re-enter"
echo "  credentials if you reinstall. Delete it manually if you no longer need it."
