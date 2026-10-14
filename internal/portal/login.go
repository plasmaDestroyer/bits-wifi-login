package portal

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/plasmaDestroyer/bits-wifi-login/internal/creds"
)

// Session is what the login learns. No timeout: the keepalive page advertises
// 14400s and it is lying. The real lifetime is measured, see session.Observe.
type Session struct {
	LoginAt time.Time
}

// ErrAlreadyOnline means the probe answered 204 on the way to logging in: a
// blip healed by itself, so there is nothing to log into.
var ErrAlreadyOnline = errors.New("portal: already online")

func (p *Portal) Login(c creds.Creds) (Session, error) {
	magic, which, ok := p.magicToken()

	if !ok {
		if p.interceptStatus == http.StatusNoContent {
			return Session{}, ErrAlreadyOnline
		}

		return Session{}, noMagicError(p.interceptBody, p.interceptStatus, p.interceptLocation)
	}

	// Strategy name only, never the token: these logs are world-readable.
	log.Printf("magic via %s", which)

	authUrl := p.baseURL + "/fgtauth?" + magic

	res, err := p.follow.Get(authUrl)
	if err != nil {
		return Session{}, fmt.Errorf("portal: fgtauth failed: %w", err)
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
		return Session{}, fmt.Errorf("portal: credential POST failed: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return Session{}, fmt.Errorf("portal: reading login response failed: %w", err)
	}

	token, ok := keepaliveFromBody(string(body))
	if !ok {
		return Session{}, rejectionError(body, res.StatusCode, c.Password)
	}

	keepaliveUrl := p.baseURL + "/keepalive?" + token

	res, err = p.follow.Get(keepaliveUrl)
	if err != nil {
		return Session{}, fmt.Errorf("portal: keepalive failed: %w", err)
	}
	defer res.Body.Close()

	// The keepalive GET is what activates the session. The body is junk.
	return Session{LoginAt: time.Now()}, nil
}

// rejectionError tells bad credentials from a response we don't understand and
// dumps the body for bug reports, password scrubbed in case the page echoes it.
func rejectionError(body []byte, status int, password string) error {
	reason := fmt.Sprintf("unexpected portal response (HTTP %d)", status)
	if rejectRe.Match(body) {
		reason = "portal rejected credentials — wrong username or password"
	}

	path, err := dumpBody(redact(body, password))
	if err != nil {
		return fmt.Errorf("portal: %s", reason)
	}

	return fmt.Errorf("portal: %s (response saved to %s — attach when reporting a bug)", reason, path)
}

func redact(body []byte, password string) []byte {
	if password == "" {
		return body
	}

	return bytes.ReplaceAll(body, []byte(password), []byte("[REDACTED]"))
}

// noMagicError dumps what the probe actually got, so the next failure comes
// with evidence instead of a shrug.
func noMagicError(interceptBody []byte, status int, location string) error {
	if len(interceptBody) == 0 {
		return fmt.Errorf("portal: no magic token found — the probe returned HTTP %d with an empty body and Location %q. If a VPN is up (CloudflareWARP, Tailscale) it is tunnelling the probe past the captive portal; exclude %s from it", status, location, connectivityURL)
	}

	path, err := dumpBody(interceptBody)
	if err != nil {
		return errors.New("portal: no magic token found")
	}

	return fmt.Errorf("portal: no magic token found (HTTP %d) — the probe response was saved to %s, attach it when reporting this", status, path)
}

func dumpBody(body []byte) (string, error) {
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
	// Reuse IsLoggedIn's probe once. Retries need a fresh one.
	if !p.fresh {
		if _, err := p.probe(); err != nil {
			return "", "", false
		}
	}
	p.fresh = false

	if token, ok := magicFromRedirect(p.interceptLocation); ok {
		return token, "redirect", true
	}

	// THE live path: BITS answers the probe with a body pointing at
	// /fgtauth?<magic>, no Location header. Never prune this (2026-08-08).
	body := string(p.interceptBody)

	if token, ok := magicFromRedirect(body); ok {
		return token, "body", true
	}
	if token, ok := magicFromForm(body); ok {
		return token, "body-form", true
	}

	// No GET / fallback: the real portal empty-replies there.
	return "", "", false
}
