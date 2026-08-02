#!/usr/bin/env bash
# Tests the token extraction regexes from fortinet-login.sh against fixture HTML.
# Run from the repo root: bash tests/test_token_extraction.sh

set -euo pipefail

FIXTURES_DIR="$(cd "$(dirname "$0")/fixtures" && pwd)"
PASS=0
FAIL=0

assert_eq() {
    local desc="$1" expected="$2" actual="$3"
    if [[ "$expected" == "$actual" ]]; then
        echo "  PASS: $desc"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $desc"
        echo "        expected: '$expected'"
        echo "        actual:   '$actual'"
        FAIL=$((FAIL + 1))
    fi
}

echo "Token extraction tests"
echo "======================"

# Strategy 1: fgtauth? in a redirect URL (simulates curl --write-out %{redirect_url})
REDIRECT_URL="https://fw.bits-pilani.ac.in:8090/fgtauth?deadbeef12345678"
token=$(echo "$REDIRECT_URL" | grep -ioE 'fgtauth\?[a-f0-9]+' | cut -d? -f2 || true)
assert_eq "strategy 1: fgtauth? in redirect URL" "deadbeef12345678" "$token"

# Strategy 2a: magic= in intercept body
token=$(grep -ioE '(magic=|fgtauth\?)[a-f0-9]+' "$FIXTURES_DIR/intercept_magic.html" | head -n 1 | awk -F'[?=]' '{print $2}' || true)
assert_eq "strategy 2a: magic= in intercept body" "deadbeef12345678" "$token"

# Strategy 2b: fgtauth? in intercept body
token=$(grep -ioE '(magic=|fgtauth\?)[a-f0-9]+' "$FIXTURES_DIR/intercept_fgtauth.html" | head -n 1 | awk -F'[?=]' '{print $2}' || true)
assert_eq "strategy 2b: fgtauth? in intercept body" "deadbeef12345678" "$token"

# Strategy 3: name="magic" hidden input in portal form
token=$(grep -ioE 'name="magic"[[:space:]]+value="?[a-f0-9]+' "$FIXTURES_DIR/portal_form.html" | grep -ioE '[a-f0-9]+$' | head -n 1 || true)
assert_eq "strategy 3: name=magic hidden input in portal form" "deadbeef12345678" "$token"

# Keepalive token present on successful login
keepalive=$(grep -m 1 -ioE 'keepalive\?[a-f0-9]+' "$FIXTURES_DIR/login_success.html" | cut -d? -f2 || true)
assert_eq "keepalive token extracted on success" "cafebabe87654321" "$keepalive"

# No keepalive token on failed login
keepalive=$(grep -m 1 -ioE 'keepalive\?[a-f0-9]+' "$FIXTURES_DIR/login_failure.html" | cut -d? -f2 || true)
assert_eq "no keepalive token on failure" "" "$keepalive"

# TLS verification stays on unless BITS_INSECURE=1 (issue #9): no hardcoded -k
ROOT_DIR="$(dirname "$(dirname "$(readlink -f "$0")")")"
hardcoded=$(grep -hE 'curl(\.exe)? ' "$ROOT_DIR/fortinet-login.sh" "$ROOT_DIR/windows/fortinet-login.ps1" |
    grep -cE '(^| )-[a-zA-Z]*k' || true)
assert_eq "no hardcoded -k in curl calls" "0" "$hardcoded"

echo ""
echo "Results: $PASS passed, $FAIL failed"
[[ $FAIL -eq 0 ]]
