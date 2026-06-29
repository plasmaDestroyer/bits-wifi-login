package portal

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMagicToken(t *testing.T) {
	cases := []struct {
		name   string
		handler http.HandlerFunc
		wantToken string
		wantWhich string
		wantOk  bool
	}{
		{
			name: "redirect",
			handler: func(w http.ResponseWriter, r *http.Request) {
    			w.Header().Set("Location", "/fgtauth?deadbeef12345678")
       			w.WriteHeader(http.StatusFound)
			},
			wantToken: "deadbeef12345678",
			wantWhich: "redirect",
			wantOk: true,
		},
		{
			name: "body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("var magic=deadbeef12345678;"))
			},
			wantToken: "deadbeef12345678",
			wantWhich: "body",
			wantOk:    true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(c.handler)
			defer srv.Close()

			client := srv.Client()
			client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}

			p := Portal{
				client:          client,
				connectivityURL: srv.URL,
			}

			token, which, ok := p.magicToken()
			if token != c.wantToken || which != c.wantWhich || ok != c.wantOk {
				t.Errorf("magicToken() = (%q, %q, %v), want (%q, %q, %v)",
					token, which, ok, c.wantToken, c.wantWhich, c.wantOk)
			}
		})
	}
}
