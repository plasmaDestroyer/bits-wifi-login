package portal

import "regexp"

var redirectRe = regexp.MustCompile(`fgtauth\?([a-f0-9]+)`)
var bodyRe = regexp.MustCompile(`(?:magic=|fgtauth\?)([a-f0-9]+)`)
var formRe = regexp.MustCompile(`name="magic"[[:space:]]+value="?([a-f0-9]+)`)
var keepaliveRe = regexp.MustCompile(`keepalive\?([a-f0-9]+)`)

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
