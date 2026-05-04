#!/usr/bin/env bash

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT_PATH="${SCRIPT_DIR}/fortinet-login.sh"
USERNAME="$(whoami)"

log() { echo "[$(date '+%H:%M:%S')] $*"; }

# ── Preflight checks ──────────────────────────────────────────────────────────

if [[ ! -f "$SCRIPT_PATH" ]]; then
    log "ERROR: fortinet-login.sh not found at $SCRIPT_PATH"
    exit 1
fi

if ! command -v nmcli &>/dev/null; then
    log "ERROR: NetworkManager not found. Is this an NM-managed system?"
    exit 1
fi

# ── Credentials ───────────────────────────────────────────────────────────────

if [[ ! -f "${SCRIPT_DIR}/creds.conf" ]]; then
    log "No creds.conf found. Let's create one."
    read -rp "Enter your BITS username: " input_user </dev/tty
    read -rsp "Enter your BITS password: " input_pass </dev/tty
    echo ""
    printf "USERNAME=%q\n" "$input_user" > "${SCRIPT_DIR}/creds.conf"
    printf "PASSWORD=%q\n" "$input_pass" >> "${SCRIPT_DIR}/creds.conf"
    chmod 600 "${SCRIPT_DIR}/creds.conf"
    log "✓ creds.conf created."
else
    log "✓ creds.conf already exists, skipping."
fi

# ── Make script executable ────────────────────────────────────────────────────

chmod +x "$SCRIPT_PATH"
log "✓ Script permissions set."

# ── NetworkManager dispatcher ─────────────────────────────────────────────────

sudo tee /etc/NetworkManager/dispatcher.d/90-fortinet-login > /dev/null << EOF
#!/usr/bin/env bash
CURRENT_SSID=\$(nmcli -t -f active,ssid dev wifi 2>/dev/null | grep '^yes' | cut -d: -f2)
if [[ "\$2" == "up" ]] && [[ "\$CURRENT_SSID" =~ ^BITS-(STUDENT|STAFF)$ ]]; then
    sleep 3
    su -c "${SCRIPT_PATH} >> /tmp/fortinet-nm-${USERNAME}.log 2>&1 &" ${USERNAME}
fi
EOF
sudo chmod +x /etc/NetworkManager/dispatcher.d/90-fortinet-login
log "✓ NetworkManager dispatcher installed."

# ── Suspend/resume hook ───────────────────────────────────────────────────────

# Handle differing distro paths for system-sleep
SLEEP_DIR="/usr/lib/systemd/system-sleep"
if [[ -d "/lib/systemd/system-sleep" && ! -d "/usr/lib/systemd/system-sleep" ]]; then
    SLEEP_DIR="/lib/systemd/system-sleep"
fi
sudo mkdir -p "$SLEEP_DIR"

sudo tee "$SLEEP_DIR/bits-wifi-login" > /dev/null << EOF
#!/usr/bin/env bash
# Re-authenticate after waking from suspend (session expires after ~4 hrs)
if [[ "\$1" == "post" ]]; then
    sleep 5  # give NetworkManager time to reconnect
    CURRENT_SSID=\$(nmcli -t -f active,ssid dev wifi 2>/dev/null | grep '^yes' | cut -d: -f2)
    if [[ "\$CURRENT_SSID" =~ ^BITS-(STUDENT|STAFF)$ ]]; then
        su -c "${SCRIPT_PATH} >> /tmp/fortinet-login-${USERNAME}.log 2>&1 &" ${USERNAME}
    fi
fi
EOF
sudo chmod +x "$SLEEP_DIR/bits-wifi-login"
log "✓ Suspend/resume hook installed."

# ── systemd service ───────────────────────────────────────────────────────────

sudo tee /etc/systemd/system/bits-wifi-login.service > /dev/null << EOF
[Unit]
Description=BITS WiFi Fortinet Login
After=network-online.target

[Service]
Type=oneshot
User=${USERNAME}
ExecStart=${SCRIPT_PATH}
EOF
log "✓ systemd service installed."

# ── systemd timer (every 30 min for session expiry) ───────────────────────────

sudo tee /etc/systemd/system/bits-wifi-login.timer > /dev/null << EOF
[Unit]
Description=BITS WiFi Login periodic check

[Timer]
OnBootSec=30s
OnUnitActiveSec=30min
Persistent=true

[Install]
WantedBy=timers.target
EOF
log "✓ systemd timer installed."

# ── Enable and start ──────────────────────────────────────────────────────────

sudo systemctl daemon-reload
sudo systemctl enable --now bits-wifi-login.timer
log "✓ Timer enabled and started."

# ── Done ──────────────────────────────────────────────────────────────────────

echo ""
echo "✓ Installation complete."
echo ""
echo "  Triggers:"
echo "    - Every WiFi connect to BITS-STUDENT (NM dispatcher)"
echo "    - Every resume from suspend/sleep (system-sleep hook)"
echo "    - Every 30 minutes (systemd timer, persistent across sleep)"
echo ""
echo "  Logs:"
echo "    journalctl -u bits-wifi-login.service --since today"
echo "    tail /tmp/fortinet-nm-${USERNAME}.log   # NM dispatcher log"
