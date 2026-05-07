#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT_PATH="${SCRIPT_DIR}/fortinet-login.sh"
USERNAME="$(whoami)"

log() { echo "[$(date '+%H:%M:%S')] $*"; }

escape_creds_value() {
    local value="$1"

    value="${value//\\/\\\\}"
    value="${value//\"/\\\"}"
    value="${value//$'\r'/\\r}"
    value="${value//$'\n'/\\n}"
    value="${value//$'\t'/\\t}"

    printf '"%s"\n' "$value"
}

fail_hint() {
    log "ERROR: install failed at line $1."
    log "Some files may have been installed already. Re-run after fixing the error, or uninstall manually."
}

trap 'fail_hint "$LINENO"' ERR

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
    {
        printf "USERNAME="
        escape_creds_value "$input_user"
        printf "PASSWORD="
        escape_creds_value "$input_pass"
    } > "${SCRIPT_DIR}/creds.conf"
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
    wait_for_network() {
        local tries=0
        until curl -sk --max-time 3 -o /dev/null -w "%{http_code}" \
            "http://connectivitycheck.gstatic.com/generate_204" \
            | grep -q "204\|302"; do
            tries=\$((tries + 1))
            [[ \$tries -ge 10 ]] && return 1
            sleep 3
        done
    }
    wait_for_network && su -c "${SCRIPT_PATH} >> /tmp/fortinet-nm-${USERNAME}.log 2>&1" ${USERNAME}
fi
EOF

sudo chmod +x /etc/NetworkManager/dispatcher.d/90-fortinet-login
sudo test -x /etc/NetworkManager/dispatcher.d/90-fortinet-login
log "✓ NetworkManager dispatcher installed."

# ── Resume service ────────────────────────────────────────────────────────────

sudo tee /etc/systemd/system/bits-wifi-login-resume.service > /dev/null << EOF
[Unit]
Description=BITS WiFi Login after resume
After=suspend.target hibernate.target hybrid-sleep.target suspend-then-hibernate.target
Wants=network-online.target

[Service]
Type=oneshot
User=${USERNAME}
ExecStartPre=/bin/sleep 5
ExecStart=${SCRIPT_PATH}

[Install]
WantedBy=suspend.target hibernate.target hybrid-sleep.target suspend-then-hibernate.target
EOF

sudo test -f /etc/systemd/system/bits-wifi-login-resume.service
log "✓ Resume service installed."

# ── systemd service ───────────────────────────────────────────────────────────

sudo tee /etc/systemd/system/bits-wifi-login.service > /dev/null << EOF
[Unit]
Description=BITS WiFi Fortinet Login
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
User=${USERNAME}
ExecStart=${SCRIPT_PATH}

[Install]
WantedBy=multi-user.target
EOF

sudo test -f /etc/systemd/system/bits-wifi-login.service
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

sudo test -f /etc/systemd/system/bits-wifi-login.timer
log "✓ systemd timer installed."

# ── Enable and start ──────────────────────────────────────────────────────────

sudo systemctl daemon-reload
sudo systemctl enable bits-wifi-login-resume.service
sudo systemctl enable --now bits-wifi-login.timer
sudo systemctl is-enabled bits-wifi-login-resume.service >/dev/null
sudo systemctl is-enabled bits-wifi-login.timer >/dev/null
log "✓ Timer enabled and started."

# ── Done ──────────────────────────────────────────────────────────────────────

echo ""
echo "✓ Installation complete."
echo ""
echo "  Triggers:"
echo "    - Every WiFi connect to BITS-STUDENT (NM dispatcher)"
echo "    - Every resume from suspend/sleep (systemd resume service)"
echo "    - Every 30 minutes (systemd timer, persistent across sleep)"
echo ""
echo "  Logs:"
echo "    journalctl -u bits-wifi-login.service --since today"
echo "    tail /tmp/fortinet-nm-${USERNAME}.log   # NM dispatcher log"
