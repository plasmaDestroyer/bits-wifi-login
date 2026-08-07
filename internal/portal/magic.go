package portal

import "regexp"

// The separator is `?` in stock FortiGate (`/fgtauth?<magic>` — the magic is a
// bare query string, not a key=value pair), but this has never been read off the
// real BITS portal, so accept `=` too rather than fail closed on a guess.
var redirectRe = regexp.MustCompile(`fgtauth[?=]([a-f0-9]+)`)
var bodyRe = regexp.MustCompile(`(?:magic=|fgtauth[?=])([a-f0-9]+)`)
var formRe = regexp.MustCompile(`name="magic"[[:space:]]+value="?([a-f0-9]+)`)
var keepaliveRe = regexp.MustCompile(`keepalive\?([a-f0-9]+)`)
var rejectRe = regexp.MustCompile(`(?i)invalid.{0,30}(credential|password|user)|wrong.{0,20}(password|user)|authentication.{0,20}fail|please.{0,10}try.{0,10}again|login.{0,10}fail`)

func magicFromRedirect(redirectURL string) (string, bool) {
	return firstSubmatch(redirectRe, redirectURL)
}

func magicFromBody(body string) (string, bool) {
	return firstSubmatch(bodyRe, body)
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
