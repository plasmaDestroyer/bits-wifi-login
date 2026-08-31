package portal

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"
)

const (
	portalURL       = "https://fw.bits-pilani.ac.in:8090"
	connectivityURL = "http://connectivitycheck.gstatic.com/generate_204"
	timeout         = 10 * time.Second
)

// probeTimeout bounds the connectivity probe alone, not the login round trips —
// the portal is slow and flaky enough on those to want the full 10s.
//
// A working network answers generate_204 in well under 100ms, so 10s on the
// probe only ever costs: it is paid in full precisely when the session has just
// dropped and the request hangs. Measured 2026-08-31, that was most of a 23s
// recovery, since both IsLoggedIn and the token probe pay it back to back.
//
// Timing out early on a genuinely slow link is safe rather than harmful: the
// login that follows re-probes, gets a 204, and returns ErrAlreadyOnline.
//
// A var, not a const, so tests need not sleep for real seconds.
var probeTimeout = 3 * time.Second

type Portal struct {
	noFollow        *http.Client
	follow          *http.Client
	connectivityURL string
	baseURL         string

	// interceptBody is what the probe answered with, kept so a failed token
	// extraction can dump the evidence instead of just saying "not found".
	interceptBody []byte
	// interceptLocation is the probe's Location header, kept for the same reason.
	interceptLocation string
	// interceptStatus is the probe's HTTP status. An empty intercept body says
	// nothing on its own — a VPN swallowing the probe and a portal that changed
	// its page look identical without it. CloudflareWARP tunnels the probe past
	// the Fortinet in QUIC, which is exactly this failure.
	interceptStatus int
	// fresh marks a probe result that magicToken has not consumed yet.
	fresh bool
}

// New builds a Portal aimed at the real BITS captive portal. Both clients share
// one cookie jar: the portal sets a session cookie on GET /fgtauth?<magic> and
// expects it back on the credential POST, which a different client issues.
func New() *Portal {
	jar, _ := cookiejar.New(nil)

	return &Portal{
		noFollow: &http.Client{
			Jar:     jar,
			Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		follow:          &http.Client{Jar: jar, Timeout: timeout},
		connectivityURL: connectivityURL,
		baseURL:         portalURL,
	}
}

// probe makes the connectivity check request once and remembers the answer. The
// login flow needs that same response twice — to decide whether there is
// anything to do, and then to pull the magic token out of the intercept page —
// and asking twice paid for a second round trip, up to a whole timeout, in the
// one situation where the latency is visible: someone sitting at a dead network
// waiting to be let back on.
func (p *Portal) probe() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.connectivityURL, nil)
	if err != nil {
		p.fresh = false
		return 0, err
	}

	res, err := p.noFollow.Do(req)
	if err != nil {
		p.fresh = false
		return 0, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		p.fresh = false
		return 0, err
	}

	p.interceptLocation = res.Header.Get("Location")
	p.interceptStatus = res.StatusCode
	p.interceptBody = body
	p.fresh = true

	return res.StatusCode, nil
}

func (p *Portal) IsLoggedIn() bool {
	status, err := p.probe()

	return err == nil && status == http.StatusNoContent
}
