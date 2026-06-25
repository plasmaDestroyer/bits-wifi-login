package portal

import "regexp"

var redirectRe = regexp.MustCompile(`fgtauth\?([a-f0-9]+)`)
var bodyRe = regexp.MustCompile(`(?:magic=|fgtauth\?)([a-f0-9]+)`)

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
