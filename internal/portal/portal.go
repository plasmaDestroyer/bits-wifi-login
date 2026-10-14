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

// Probe only. Login round trips keep the full 10s. A live network answers in
// ~84ms, so this is only paid when the session just died. A var so tests
// don't sleep for real.
var probeTimeout = time.Second

type Portal struct {
	noFollow        *http.Client
	follow          *http.Client
	connectivityURL string
	baseURL         string

	// The last probe's answer, kept for token extraction and failure dumps.
	// Status matters: an empty body alone can't tell a VPN from a changed portal.
	interceptBody     []byte
	interceptLocation string
	interceptStatus   int
	fresh             bool // not yet consumed by magicToken
}

// New builds a Portal for the real BITS portal. Both clients MUST share one
// cookie jar: the cookie set on GET /fgtauth?<magic> has to come back on the
// credential POST, which the other client sends.
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

// probe makes the connectivity check and remembers the answer for magicToken.
func (p *Portal) probe() (int, error) {
	// Cleared first: a leftover 204 once made a real expiry look "already online".
	p.interceptStatus, p.interceptBody, p.interceptLocation, p.fresh = 0, nil, "", false

	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.connectivityURL, nil)
	if err != nil {
		return 0, err
	}

	res, err := p.noFollow.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
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
