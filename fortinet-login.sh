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
    curl -c /tmp/fortinet_cookies.txt -b /tmp/fortinet_cookies.txt -skL \
        --max-time 10 -i "http://detectportal.firefox.com/canonical.html" \
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

    # Emulate browser: GET the form page to initialize session server-side
    curl -c /tmp/fortinet_cookies.txt -b /tmp/fortinet_cookies.txt -skL \
         "${PORTAL}/fgtauth?${magic}" -o /dev/null

    # Emulate browser: Submit the form to / (exactly as the form action="/" specifies)
    log "Submitting credentials..."
    local post_resp
    post_resp=$(curl -c /tmp/fortinet_cookies.txt -b /tmp/fortinet_cookies.txt -sk \
        -X POST \
        "${PORTAL}/" \
        --data-urlencode "username=${USERNAME}" \
        --data-urlencode "password=${PASSWORD}" \
        --data "magic=${magic}" \
        --data "4Tredir=http://detectportal.firefox.com/canonical.html")

    # Emulate browser: Follow the JavaScript redirect to the keepalive endpoint
    local keepalive
    keepalive=$(echo "$post_resp" | grep -m 1 -ioP 'keepalive\?\K[a-f0-9]+')

    if [[ -n "$keepalive" ]]; then
        log "Credentials accepted! Found keepalive logic, activating connection..."
        curl -c /tmp/fortinet_cookies.txt -b /tmp/fortinet_cookies.txt -skL \
            "${PORTAL}/keepalive?${keepalive}" -o /dev/null
    else
        log "Warning: No keepalive redirect found. Fortinet might have rejected the login."
        echo "$post_resp" > /tmp/fortinet_error.html
    fi

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
