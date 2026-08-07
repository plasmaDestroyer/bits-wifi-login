package portal

import (
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

func (p *Portal) IsLoggedIn() bool {
	res, err := p.noFollow.Get(p.connectivityURL)
	if err != nil {
		return false
	}
	defer res.Body.Close()

	return res.StatusCode == http.StatusNoContent
}
