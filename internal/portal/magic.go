package portal

import (
	"regexp"
	"strconv"
	"time"
)

// Separator confirmed `?` against the live portal on 2026-08-08: the magic is a
// bare query string, not a key=value pair.
//
//	https://fw.bits-pilani.ac.in:8090/fgtauth?034ea02187c4c8d7
//	https://fw.bits-pilani.ac.in:8090/keepalive?0e050d0a05030901
var redirectRe = regexp.MustCompile(`fgtauth\?([a-f0-9]+)`)
var formRe = regexp.MustCompile(`name="magic"[[:space:]]+value="?([a-f0-9]+)`)
var keepaliveRe = regexp.MustCompile(`keepalive\?([a-f0-9]+)`)
var rejectRe = regexp.MustCompile(`(?i)invalid.{0,30}(credential|password|user)|wrong.{0,20}(password|user)|authentication.{0,20}fail|please.{0,10}try.{0,10}again|login.{0,10}fail`)

// The portal renders its configured auth-timeout into the keepalive page:
//
//	<p>Authentication refresh in <b id="countdown">14400</b> seconds ...</p>
//
// Parsed rather than hardcoded so a change on the Fortinet side is picked up for
// free. Note this is the *total* timeout, never the time remaining — the page's
// countdown is computed client-side from page load, so it reads 14400 on every
// fetch no matter how old the session is.
var countdownRe = regexp.MustCompile(`id="countdown">\s*(\d+)`)

func magicFromRedirect(redirectURL string) (string, bool) {
	return firstSubmatch(redirectRe, redirectURL)
}

func magicFromForm(html string) (string, bool) {
	return firstSubmatch(formRe, html)
}

func keepaliveFromBody(body string) (string, bool) {
	return firstSubmatch(keepaliveRe, body)
}

func timeoutFromBody(body string) (time.Duration, bool) {
	capture, ok := firstSubmatch(countdownRe, body)
	if !ok {
		return 0, false
	}

	seconds, err := strconv.Atoi(capture)
	if err != nil || seconds <= 0 {
		return 0, false
	}

	return time.Duration(seconds) * time.Second, true
}

func firstSubmatch(re *regexp.Regexp, s string) (string, bool) {
	capture := re.FindStringSubmatch(s)

	if capture == nil {
		return "", false
	}

	return capture[1], true
}
