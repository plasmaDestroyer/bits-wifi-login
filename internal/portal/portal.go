package portal

import (
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
	res, err := p.noFollow.Get(p.connectivityURL)
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
	p.interceptBody = body
	p.fresh = true

	return res.StatusCode, nil
}

func (p *Portal) IsLoggedIn() bool {
	status, err := p.probe()

	return err == nil && status == http.StatusNoContent
}
