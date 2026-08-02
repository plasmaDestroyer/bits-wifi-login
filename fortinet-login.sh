#!/usr/bin/env bash

log() { echo "[$(date '+%H:%M:%S')] $*"; }

# ─── CONFIG ───────────────────────────────────────────────────────────────────
if [[ "$OSTYPE" == "darwin"* ]]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
else
    SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"
fi
CREDS_FILE="${SCRIPT_DIR}/creds.conf"
PORTAL="https://fw.bits-pilani.ac.in:8090"

# TLS verification is ON by default — credentials are POSTed to the portal.
# If the portal certificate is not trusted by this machine, set BITS_INSECURE=1.
# ponytail: env escape hatch beats pinning; swap for --cacert if a CA ever ships.
TLS_OPT=()
if [[ "${BITS_INSECURE:-0}" == "1" ]]; then
    TLS_OPT=(-k)
fi

tls_hint() {
    [[ "${BITS_INSECURE:-0}" == "1" ]] && return 0
    log "  If the portal certificate is untrusted, retry with BITS_INSECURE=1 (disables TLS verification)."
}

# Set strict permissions for newly created sensitive files (cookies, error logs)
umask 077

COOKIE_FILE="$(mktemp "/tmp/fortinet_cookies_$(id -u).XXXXXX")" || {
    log "ERROR: Could not create temporary cookie file."
    exit 1
}
trap 'rm -f "$COOKIE_FILE"' EXIT

# ─── CREDENTIALS ──────────────────────────────────────────────────────────────
unescape_creds_value() {
    local value="$1"
    local result=""
    local i char next

    for ((i = 0; i < ${#value}; i++)); do
        char="${value:i:1}"
        if [[ "$char" == "\\" && $((i + 1)) -lt ${#value} ]]; then
            i=$((i + 1))
            next="${value:i:1}"
            case "$next" in
            "\\") result+="\\" ;;
            '"') result+='"' ;;
            n) result+=$'\n' ;;
            r) result+=$'\r' ;;
            t) result+=$'\t' ;;
            *) result+="$next" ;;
            esac
        else
            result+="$char"
        fi
    done

    printf '%s' "$result"
}

read_creds_value() {
    local key="$1"
    local line value

    line="$(grep -E "^[[:space:]]*${key}[[:space:]]*=" "$CREDS_FILE" | tail -n 1 || true)"
    [[ -n "$line" ]] || return 0

    value="${line#*=}"
    value="${value#"${value%%[![:space:]]*}"}"
    value="${value%"${value##*[![:space:]]}"}"

    if [[ "$value" == \"*\" && "$value" == *\" ]]; then
        value="${value:1:${#value}-2}"
        unescape_creds_value "$value"
    elif [[ "$value" == \'*\' && "$value" == *\' ]]; then
        printf '%s' "${value:1:${#value}-2}"
    else
        printf '%s' "$value"
    fi
}

if [[ ! -f "$CREDS_FILE" ]]; then
    log "ERROR: Credentials file not found at $CREDS_FILE"
    exit 1
fi
chmod 600 "$CREDS_FILE" 2>/dev/null
USERNAME="$(read_creds_value USERNAME)"
PASSWORD="$(read_creds_value PASSWORD)"

if [[ -z "$USERNAME" || -z "$PASSWORD" ]]; then
    log "ERROR: creds.conf must contain USERNAME and PASSWORD."
    exit 1
fi

# ─── FUNCTIONS ────────────────────────────────────────────────────────────────

is_bits_ssid() {
    [[ "$SSID" =~ ^BITS-(STUDENT|STAFF)$ ]]
}

is_logged_in() {
    local code
    code=$(curl -s "${TLS_OPT[@]}" --max-time 5 -o /dev/null -w "%{http_code}" \
        "http://connectivitycheck.gstatic.com/generate_204")
    [[ "$code" == "204" ]]
}

get_magic_token() {
    local magic
    # 1. Try captive portal redirect URL
    magic=$(curl -s "${TLS_OPT[@]}" --max-time 10 -o /dev/null -w "%{redirect_url}" "http://connectivitycheck.gstatic.com/generate_204" | grep -ioE 'fgtauth\?[a-f0-9]+' | cut -d? -f2 || true)

    # 2. Try HTML body of the intercept page (if Fortinet returns 200 OK + meta refresh)
    if [[ -z "$magic" ]]; then
        magic=$(curl -s "${TLS_OPT[@]}" --max-time 10 "http://connectivitycheck.gstatic.com/generate_204" | grep -ioE '(magic=|fgtauth\?)[a-f0-9]+' | head -n 1 | awk -F'[?=]' '{print $2}' || true)
    fi

    # 3. Fallback: Hit the portal directly and scrape the hidden input value
    if [[ -z "$magic" ]]; then
        magic=$(curl -s "${TLS_OPT[@]}" --max-time 10 "${PORTAL}/" | grep -ioE 'name="magic"[[:space:]]+value="?[a-f0-9]+' | grep -ioE '[a-f0-9]+$' | head -n 1 || true)
    fi

    echo "$magic"
}

login() {
    local magic
    magic=$(get_magic_token)

    if [[ -z "$magic" ]]; then
        log "Could not get magic token — captive portal not detected (already logged in, or portal unreachable)."
        tls_hint
        return 1
    fi

    log "Got magic token: $magic"

    # Emulate browser: GET the form page to initialize session server-side
    curl -c "$COOKIE_FILE" -b "$COOKIE_FILE" -sL "${TLS_OPT[@]}" \
        "${PORTAL}/fgtauth?${magic}" -o /dev/null

    # Emulate browser: Submit the form to / (exactly as the form action="/" specifies)
    log "Submitting credentials..."
    local post_resp http_code post_body
    post_body=$(mktemp)
    http_code=$(curl -c "$COOKIE_FILE" -b "$COOKIE_FILE" -s "${TLS_OPT[@]}" \
        -X POST \
        "${PORTAL}/" \
        --data-urlencode "username=${USERNAME}" \
        --data-urlencode "password=${PASSWORD}" \
        --data "magic=${magic}" \
        --data "4Tredir=http://connectivitycheck.gstatic.com/generate_204" \
        -o "$post_body" \
        -w '%{http_code}')
    post_resp=$(cat "$post_body")
    rm -f "$post_body"
    log "Portal responded: HTTP $http_code"

    # Emulate browser: Follow the JavaScript redirect to the keepalive endpoint
    local keepalive error_file
    keepalive=$(echo "$post_resp" | grep -m 1 -ioE 'keepalive\?[a-f0-9]+' | cut -d? -f2)
    error_file="/tmp/fortinet_error_$(id -u).html"

    if [[ -n "$keepalive" ]]; then
        log "Credentials accepted! Found keepalive logic, activating connection..."
        curl -c "$COOKIE_FILE" -b "$COOKIE_FILE" -sL "${TLS_OPT[@]}" \
            "${PORTAL}/keepalive?${keepalive}" -o /dev/null
    else
        echo "$post_resp" >"$error_file"
        if echo "$post_resp" | grep -qiE 'invalid.{0,30}(credential|password|user)|wrong.{0,20}(password|user)|authentication.{0,20}fail|please.{0,10}try.{0,10}again|login.{0,10}fail'; then
            log "✗ Portal rejected credentials — wrong username or password."
        else
            log "✗ No keepalive found — unexpected portal response (HTTP $http_code)."
            [[ "$http_code" == "000" ]] && tls_hint
        fi
        log "  Response saved to $error_file — attach when reporting a bug."
    fi

    sleep 2

    if is_logged_in; then
        log "✓ Login successful!"
        return 0
    else
        log "✗ Login failed."
        [[ -f "$error_file" ]] && log "  See $error_file for the portal response."
        return 1
    fi
}

main() {
    if command -v nmcli >/dev/null 2>&1; then
        SSID="$(nmcli -t -f active,ssid dev wifi 2>/dev/null | grep '^yes' | cut -d: -f2)"
    elif command -v networksetup >/dev/null 2>&1; then
        # Get macOS Wi-Fi interface (typically en0) and extract active network
        WIFI_IFACE="$(networksetup -listallhardwareports 2>/dev/null | awk '/Wi-Fi|AirPort/{getline; print $NF}')"
        SSID="$(networksetup -getairportnetwork "$WIFI_IFACE" 2>/dev/null | awk -F': ' '{print $2}')"
    else
        SSID="Unknown"
    fi

    if ! is_bits_ssid; then
        log "Current WiFi is '${SSID}'; not a BITS network. Skipping."
        exit 0
    fi

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
