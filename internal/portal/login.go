package portal

import "io"

func (p *Portal) magicToken() (string, string, bool) {
	res, err := p.client.Get(p.connectivityURL)
	if err != nil {
		return "", "", false
	}
	defer res.Body.Close()

	if token, ok := magicFromRedirect(res.Header.Get("Location")); ok {
		return token, "redirect", true
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", "", false
	}
	if token, ok := magicFromBody(string(body)); ok {
		return token, "body", true
	}

	return "", "", false
}
