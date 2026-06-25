package portal

import "net/http"

type Portal struct {
	client          *http.Client
	connectivityURL string
	baseURL         string
}

func (p *Portal) IsLoggedIn() bool {
	res, err := p.client.Get(p.connectivityURL)
	if err != nil {
		return false
	}
	defer res.Body.Close()

	return res.StatusCode == http.StatusNoContent
}
