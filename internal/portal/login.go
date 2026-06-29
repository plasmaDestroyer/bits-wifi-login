package portal

func (p *Portal) magicToken() (string, string, bool) {
	res, err := p.client.Get(p.connectivityURL)
	if err != nil {
		return "", "", false
	}
	defer res.Body.Close()


	if token, ok := magicFromRedirect(res.Header.Get("Location")); ok {
		return token, "redirect", true
	}

	return "", "", false
}
