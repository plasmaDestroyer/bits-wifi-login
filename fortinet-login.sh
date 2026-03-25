#!/usr/bin/env bash

log() { echo "[$(date '+%H:%M:%S')] $*"; }

# ─── CONFIG ───────────────────────────────────────────────────────────────────
CREDS_FILE="$(dirname "$0")/creds.conf"
PORTAL="https://fw.bits-pilani.ac.in:8090"
SSID="$(nmcli -t -f active,ssid dev wifi | grep '^yes' | cut -d: -f2)"

if [[ ! -f "$CREDS_FILE" ]]; then
    log "ERROR: Credentials file not found at $CREDS_FILE"
    exit 1
fi
source "$CREDS_FILE"
# ──────────────────────────────────────────────────────────────────────────────

is_logged_in() {
    local body
    body=$(curl -sk --max-time 5 "http://detectportal.firefox.com/canonical.html")
    [[ "$body" == *"success"* ]]
}

get_magic_token() {
    # Fetch headers (-i) and body, follow redirects (-L) instead of dropping them
    # grep extracts the token part from the fgtauth URL anywhere in the response
    curl -skL --max-time 10 -i "http://detectportal.firefox.com/canonical.html" \
        | grep -m 1 -ioP 'fgtauth\?\K[a-f0-9]+'
}

login() {
    local magic
    magic=$(get_magic_token)

    if [[ -z "$magic" ]]; then
        log "Could not get magic token — are you already logged in?"
        return 1
    fi

    log "Got magic token: $magic"

    curl -sk --max-time 10 \
        -X POST \
        "${PORTAL}/" \
        --data-urlencode "username=${USERNAME}" \
        --data-urlencode "password=${PASSWORD}" \
        --data "magic=${magic}" \
        --data "4Tredir=http://detectportal.firefox.com/canonical.html" \
        -o /dev/null

    sleep 2

    if is_logged_in; then
        log "✓ Login successful!"
        return 0
    else
        log "✗ Login failed — check credentials or portal reachability."
        return 1
    fi
}

main() {
    log "Checking connectivity..."
    if is_logged_in; then
        log "Already authenticated, nothing to do."
        exit 0
    fi

    log "Not logged in. Authenticating to ${SSID}..."
    login
}

main
