package portal

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"

	"github.com/plasmaDestroyer/bits-wifi-login/internal/creds"
)

func (p *Portal) Login(c creds.Creds) error {
	magic, which, ok := p.magicToken()

	if !ok {
		return errors.New("portal: no magic token found")
	}

	authUrl := p.baseURL + "/fgtauth?" + magic

	res, err := p.follow.Get(authUrl)
	if err != nil {
		return fmt.Errorf("portal: fgtauth failed: %w", err)
	}
	defer res.Body.Close()

	res, err = p.noFollow.PostForm(p.baseURL, url.Values{
		"username": {c.Username},
		"password": {c.Password},
		"magic":    {magic},
		"4Tredir":  {p.connectivityURL},
	})
	if err != nil {
		return fmt.Errorf("portal: credential POST failed: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("portal: reading login response failed: %w", err)
	}

	token, ok := keepaliveFromBody(string(body))
	if !ok {
		return errors.New("portal: login rejected (no keepalive — wrong credentials or unexpected response)")
	}

	keepaliveUrl := p.baseURL + "/keepalive?" + token

	res, err = p.follow.Get(keepaliveUrl)
	if err != nil {
		return fmt.Errorf("portal: keepalive failed: %w", err)
	}
	defer res.Body.Close()

	log.Printf("magic via %s", which)

	return nil
}

func (p *Portal) magicToken() (string, string, bool) {
	res, err := p.noFollow.Get(p.connectivityURL)
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

	res2, err := p.noFollow.Get(p.baseURL)
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
