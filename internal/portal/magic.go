package portal

import "regexp"

var redirectRe = regexp.MustCompile(`fgtauth\?([a-f0-9]+)`)
var bodyRe = regexp.MustCompile(`(?:magic=|fgtauth\?)([a-f0-9]+)`)
var formRe = regexp.MustCompile(`name="magic"[[:space:]]+value="?([a-f0-9]+)`)
var keepaliveRe = regexp.MustCompile(`keepalive\?([a-f0-9]+)`)

func magicFromRedirect(redirectURL string) (string, bool) {
	capture := redirectRe.FindStringSubmatch(redirectURL)

	if capture == nil {
		return "", false
	}

	return capture[1], true
}

func magicFromBody(body string) (string, bool) {
	capture := bodyRe.FindStringSubmatch(body)

	if capture == nil {
		return "", false
	}

	return capture[1], true
}

func magicFromForm(html string) (string, bool) {
	capture := formRe.FindStringSubmatch(html)

	if capture == nil {
		return "", false
	}

	return capture[1], true
}

func keepaliveFromBody(body string) (string, bool) {
	capture := keepaliveRe.FindStringSubmatch(body)

	if capture == nil {
		return "", false
	}

	return capture[1], true
}
