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

// Session is what the login learns. No token is kept: the expiry is anticipated
// by watching the clock, never by asking the portal, which answers nothing about
// a live session anyway.
//
// No timeout either. The keepalive page advertises 14400s and that number is not
// the session lifetime — measured 2026-08-28, a session outlived it by hours —
// so reading it produced a confidently wrong deadline. How long a session lasts
// is measured from observed drops instead; see session.Observe.
type Session struct {
	LoginAt time.Time
}

// ErrAlreadyOnline means the probe answered 204 on the way to logging in, so
// there is nothing to log into. It happens when a transient outage heals between
// IsLoggedIn's probe failing and the token probe being made — seen twice on
// 2026-08-31, where a network hiccup at 11:37 and 12:46 sent a healthy machine
// through four login attempts and a fatal "all attempts failed".
var ErrAlreadyOnline = errors.New("portal: already online")

func (p *Portal) Login(c creds.Creds) (Session, error) {
	magic, which, ok := p.magicToken()

	if !ok {
		// Check this before reporting a missing token: a 204 is not a portal we
		// failed to parse, it is no portal at all.
		if p.interceptStatus == http.StatusNoContent {
			return Session{}, ErrAlreadyOnline
		}

		return Session{}, noMagicError(p.interceptBody, p.interceptStatus, p.interceptLocation)
	}

	// Which strategy fired is the evidence for pruning the other two — log it
	// before anything downstream can fail. The token value itself is not logged:
	// these logs land in journald and world-readable dispatcher logs, and the
	// strategy name is the only part worth keeping.
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

	// The keepalive GET above is what actually activates the session, so it has to
	// happen; its body has nothing worth reading.
	return Session{LoginAt: time.Now()}, nil
}

// rejectionError distinguishes bad credentials from a portal response we don't
// understand, and saves the body so a changed portal can be diagnosed later.
// The dump is meant to be attached to bug reports, so the password is scrubbed
// out of it first — a Fortinet failure page re-renders the login form and there
// is no guarantee it does not echo what was submitted.
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

// noMagicError saves whatever the portal actually answered the probe with. Two
// wrong guesses about that response have already cost a day of failed logins;
// the next failure should hand over evidence, not a shrug.
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
	// IsLoggedIn just made this exact request, so reuse its answer instead of
	// paying for it twice. Consumed either way: on a retry the previous body is
	// known-useless, and a stale one would only reproduce the same failure.
	if !p.fresh {
		if _, err := p.probe(); err != nil {
			return "", "", false
		}
	}
	p.fresh = false

	if token, ok := magicFromRedirect(p.interceptLocation); ok {
		return token, "redirect", true
	}

	// The intercept page itself, which is what the BITS portal actually serves:
	// it answers the probe with a body pointing at /fgtauth?<magic> rather than a
	// Location header. Confirmed 2026-08-08 — a build that only looked at the
	// header and at GET / failed every single attempt for a whole day.
	body := string(p.interceptBody)

	if token, ok := magicFromRedirect(body); ok {
		return token, "body", true
	}
	if token, ok := magicFromForm(body); ok {
		return token, "body-form", true
	}

	// There used to be a fourth strategy here that fetched the portal root and
	// looked for a form in it. It was never a net: the real portal empty-replies
	// on GET /, which is exactly why the body strategy had to exist, so it could
	// not have matched — and unlike the three above, which are regex checks on a
	// response already in hand, it paid a whole HTTP request to find that out.
	// Every login on record fired "body".
	return "", "", false
}
