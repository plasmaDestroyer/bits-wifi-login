package portal

import "net/http"

type Portal struct {
	noFollow        *http.Client
	follow          *http.Client
	connectivityURL string
	baseURL         string
}

func (p *Portal) IsLoggedIn() bool {
	res, err := p.noFollow.Get(p.connectivityURL)
	if err != nil {
		return false
	}
	defer res.Body.Close()

	return res.StatusCode == http.StatusNoContent
}
