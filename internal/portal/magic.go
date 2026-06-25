package portal

import (
	"regexp"
)

var redirectRe = regexp.MustCompile(`fgtauth\?([a-f0-9]+)`)

func magicFromRedirect(redirectURL string) (string, bool) {
	capture := redirectRe.FindStringSubmatch(redirectURL)

	if capture == nil {
		return "", false
	}

	return capture[1], true
}
