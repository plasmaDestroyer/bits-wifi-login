package portal

import (
	"errors"
	"io"

	"github.com/plasmaDestroyer/bits-wifi-login/internal/creds"
)

func (p *Portal) Login(c creds.Creds) error {
	_, _, ok := p.magicToken();
	if !ok {
		return errors.New("portal: no magic token found")
	}

	return nil
}

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

	res2, err := p.client.Get(p.baseURL)
	if err != nil {
		return "", "", false
	}
	defer res2.Body.Close()

	formBody, err := io.ReadAll(res2.Body)
	if err != nil {
		return "", "", false
	}
	if token, ok := magicFromForm(string(formBody)); ok {
		return token, "form", true
	}

	return "", "", false
}
