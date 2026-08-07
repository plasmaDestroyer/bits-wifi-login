package portal

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"

	"github.com/plasmaDestroyer/bits-wifi-login/internal/creds"
)

func (p *Portal) Login(c creds.Creds) error {
	magic, which, ok := p.magicToken()

	if !ok {
		return errors.New("portal: no magic token found")
	}

	// Which strategy fired is the evidence for pruning the other two — log it
	// before anything downstream can fail.
	log.Printf("magic via %s: %s", which, magic)

	authUrl := p.baseURL + "/fgtauth?" + magic

	res, err := p.follow.Get(authUrl)
	if err != nil {
		return fmt.Errorf("portal: fgtauth failed: %w", err)
	}
	defer res.Body.Close()

	log.Print("submitting credentials...")

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
		return rejectionError(body, res.StatusCode)
	}

	keepaliveUrl := p.baseURL + "/keepalive?" + token

	res, err = p.follow.Get(keepaliveUrl)
	if err != nil {
		return fmt.Errorf("portal: keepalive failed: %w", err)
	}
	defer res.Body.Close()

	return nil
}

// rejectionError distinguishes bad credentials from a portal response we don't
// understand, and saves the body so a changed portal can be diagnosed later.
func rejectionError(body []byte, status int) error {
	reason := fmt.Sprintf("unexpected portal response (HTTP %d)", status)
	if rejectRe.Match(body) {
		reason = "portal rejected credentials — wrong username or password"
	}

	path, err := dumpBody(body)
	if err != nil {
		return fmt.Errorf("portal: %s", reason)
	}

	return fmt.Errorf("portal: %s (response saved to %s — attach when reporting a bug)", reason, path)
}

func dumpBody(body []byte) (string, error) {
	// os.CreateTemp already creates with 0600.
	f, err := os.CreateTemp("", "fortinet_error_*.html")
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := f.Write(body); err != nil {
		return "", err
	}

	return f.Name(), nil
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
