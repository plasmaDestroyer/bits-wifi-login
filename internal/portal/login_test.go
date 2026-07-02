package portal

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/plasmaDestroyer/bits-wifi-login/internal/creds"
)

func TestLogin(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantErr bool
	}{
		{
			name: "no magic token",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("nothing useful"))
			},
			wantErr: true,
		},
		{
			name: "creds rejected",
			handler: func(w http.ResponseWriter, r *http.Request) {
				switch {
					case r.Method == http.MethodGet && r.URL.Path == "/": // probe
					w.Header().Set("Location", "/fgtauth?deadbeef12345678")
					w.WriteHeader(http.StatusFound)
					case r.URL.Path == "/fgtauth": // session init
					w.WriteHeader(http.StatusOK)
					case r.Method == http.MethodPost && r.URL.Path == "/": // creds rejected: no keepalive
					w.Write([]byte("authentication failed"))
				}
			},
			wantErr: true,
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
				noFollow:        client,
				follow: 		 &http.Client{},
				connectivityURL: srv.URL,
				baseURL:         srv.URL,
			}

			err := p.Login(creds.Creds{Username: "u", Password: "p"})
			if (err != nil) != c.wantErr {
				t.Errorf("Login() error = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

func TestLoginSuccess(t *testing.T) {
    var keepaliveHit bool
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch {
        case r.Method == http.MethodGet && r.URL.Path == "/":
            w.Header().Set("Location", "/fgtauth?deadbeef12345678")
            w.WriteHeader(http.StatusFound)
        case r.URL.Path == "/fgtauth":
            w.WriteHeader(http.StatusOK)
        case r.Method == http.MethodPost && r.URL.Path == "/":
            w.Write([]byte(`window.location="/keepalive?cafebabe87654321"`))
        case r.URL.Path == "/keepalive":
            keepaliveHit = true
            w.WriteHeader(http.StatusOK)
        }
    }))
    defer srv.Close()

    noFollow := srv.Client()
    noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
    p := Portal{noFollow: noFollow, follow: &http.Client{}, connectivityURL: srv.URL, baseURL: srv.URL}

    if err := p.Login(creds.Creds{Username: "u", Password: "p"}); err != nil {
        t.Fatalf("Login() = %v, want nil", err)
    }
    if !keepaliveHit {
        t.Error("keepalive endpoint never hit — session not activated")
    }
}

func TestMagicToken(t *testing.T) {
	cases := []struct {
		name      string
		handler   http.HandlerFunc
		wantToken string
		wantWhich string
		wantOk    bool
	}{
		{
			name: "redirect",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "/fgtauth?deadbeef12345678")
				w.WriteHeader(http.StatusFound)
			},
			wantToken: "deadbeef12345678",
			wantWhich: "redirect",
			wantOk:    true,
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
		{
			name: "form",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`<input type="hidden" name="magic" value="deadbeef12345678">`))
			},
			wantToken: "deadbeef12345678",
			wantWhich: "form",
			wantOk:    true,
		},
		{
			name: "no token",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("nothing useful here"))
			},
			wantToken: "",
			wantWhich: "",
			wantOk:    false,
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
				noFollow:        client,
				connectivityURL: srv.URL,
				baseURL:         srv.URL,
			}

			token, which, ok := p.magicToken()
			if token != c.wantToken || which != c.wantWhich || ok != c.wantOk {
				t.Errorf("magicToken() = (%q, %q, %v), want (%q, %q, %v)",
					token, which, ok, c.wantToken, c.wantWhich, c.wantOk)
			}
		})
	}
}
