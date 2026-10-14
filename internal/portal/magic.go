package portal

import (
	"regexp"
)

// Bare query strings, not key=value (live portal, 2026-08-08):
//
//	https://fw.bits-pilani.ac.in:8090/fgtauth?034ea02187c4c8d7
//	https://fw.bits-pilani.ac.in:8090/keepalive?0e050d0a05030901
var redirectRe = regexp.MustCompile(`fgtauth\?([a-f0-9]+)`)
var formRe = regexp.MustCompile(`name="magic"[[:space:]]+value="?([a-f0-9]+)`)
var keepaliveRe = regexp.MustCompile(`keepalive\?([a-f0-9]+)`)
var rejectRe = regexp.MustCompile(`(?i)invalid.{0,30}(credential|password|user)|wrong.{0,20}(password|user)|authentication.{0,20}fail|please.{0,10}try.{0,10}again|login.{0,10}fail`)

func magicFromRedirect(redirectURL string) (string, bool) {
	return firstSubmatch(redirectRe, redirectURL)
}

func magicFromForm(html string) (string, bool) {
	return firstSubmatch(formRe, html)
}

func keepaliveFromBody(body string) (string, bool) {
	return firstSubmatch(keepaliveRe, body)
}

func firstSubmatch(re *regexp.Regexp, s string) (string, bool) {
	capture := re.FindStringSubmatch(s)

	if capture == nil {
		return "", false
	}

	return capture[1], true
}
