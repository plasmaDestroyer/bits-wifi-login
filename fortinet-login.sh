#!/usr/bin/env bash

log() { echo "[$(date '+%H:%M:%S')] $*"; }

# ─── CONFIG ───────────────────────────────────────────────────────────────────
CREDS_FILE="$(dirname "$(readlink -f "$0")")/creds.conf"
PORTAL="https://fw.bits-pilani.ac.in:8090"
SSID="$(nmcli -t -f active,ssid dev wifi | grep '^yes' | cut -d: -f2)"
COOKIE_FILE="/tmp/fortinet_cookies_$(id -u).txt"

# Set strict permissions for newly created sensitive files (cookies, error logs)
umask 077

if [[ ! -f "$CREDS_FILE" ]]; then
    log "ERROR: Credentials file not found at $CREDS_FILE"
    exit 1
fi
chmod 600 "$CREDS_FILE" 2>/dev/null
source "$CREDS_FILE"
# ──────────────────────────────────────────────────────────────────────────────

is_logged_in() {
    local code
    code=$(curl -sk --max-time 5 -o /dev/null -w "%{http_code}" \
        "http://connectivitycheck.gstatic.com/generate_204")
    [[ "$code" == "204" ]]
}

get_magic_token() {
    local magic
    # 1. Try captive portal redirect URL
    magic=$(curl -sk --max-time 10 -o /dev/null -w "%{redirect_url}" "http://connectivitycheck.gstatic.com/generate_204" | grep -oP '(?<=fgtauth\?)[a-f0-9]+' || true)

    # 2. Try HTML body of the intercept page (if Fortinet returns 200 OK + meta refresh)
    if [[ -z "$magic" ]]; then
        magic=$(curl -sk --max-time 10 "http://connectivitycheck.gstatic.com/generate_204" | grep -oP '(?:magic=|fgtauth\?)\K[a-f0-9]+' | head -n 1 || true)
    fi

    # 3. Fallback: Hit the portal directly and scrape the hidden input value
    if [[ -z "$magic" ]]; then
        magic=$(curl -sk --max-time 10 "${PORTAL}/" | grep -ioP 'name="magic"\s+value="\K[a-f0-9]+' | head -n 1 || true)
    fi

    echo "$magic"
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
    curl -c "$COOKIE_FILE" -b "$COOKIE_FILE" -skL \
         "${PORTAL}/fgtauth?${magic}" -o /dev/null

    # Emulate browser: Submit the form to / (exactly as the form action="/" specifies)
    log "Submitting credentials..."
    local post_resp
    post_resp=$(curl -c "$COOKIE_FILE" -b "$COOKIE_FILE" -sk \
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
        curl -c "$COOKIE_FILE" -b "$COOKIE_FILE" -skL \
            "${PORTAL}/keepalive?${keepalive}" -o /dev/null
    else
        log "Warning: No keepalive redirect found. Fortinet might have rejected the login."
        echo "$post_resp" > "/tmp/fortinet_error_$(id -u).html"
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

    for attempt in 1 2; do
        log "Attempt ${attempt}/2..."
        login && exit 0
        sleep 3
    done

    log "✗ All attempts failed."
    exit 1
}

main

