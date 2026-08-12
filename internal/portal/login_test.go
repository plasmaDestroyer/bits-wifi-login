package portal

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

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
				follow:          &http.Client{},
				connectivityURL: srv.URL,
				baseURL:         srv.URL,
			}

			_, err := p.Login(creds.Creds{Username: "u", Password: "p"})
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

	s, err := p.Login(creds.Creds{Username: "u", Password: "p"})
	if err != nil {
		t.Fatalf("Login() = %v, want nil", err)
	}
	if !keepaliveHit {
		t.Error("keepalive endpoint never hit — session not activated")
	}
	// This keepalive page carries no countdown, so the login must still hand back a
	// usable deadline rather than a zero one that would disable the watch.
	if s.Timeout != DefaultTimeout || s.LoginAt.IsZero() {
		t.Errorf("Login() session = %+v, want the default timeout and a login time", s)
	}
}

// The real portal renders its auth-timeout into the keepalive page, and that is
// the only place the deadline can come from.
func TestLoginReadsTheTimeoutFromKeepalive(t *testing.T) {
	keepalive, err := os.ReadFile("testdata/keepalive_real.html")
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			w.Header().Set("Location", "/fgtauth?deadbeef12345678")
			w.WriteHeader(http.StatusFound)
		case r.Method == http.MethodPost && r.URL.Path == "/":
			w.Write([]byte(`window.location="/keepalive?cafebabe87654321"`))
		case r.URL.Path == "/keepalive":
			w.Write(keepalive)
		}
	}))
	defer srv.Close()

	noFollow := srv.Client()
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	p := Portal{noFollow: noFollow, follow: srv.Client(), connectivityURL: srv.URL, baseURL: srv.URL}

	s, err := p.Login(creds.Creds{Username: "u", Password: "p"})
	if err != nil {
		t.Fatalf("Login() = %v, want nil", err)
	}
	if want := 14400 * time.Second; s.Timeout != want {
		t.Errorf("Login() timeout = %v, want %v", s.Timeout, want)
	}
}

func TestRejectionError(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantSubstr string
	}{
		{"wrong creds", "<p>Invalid username or password</p>", "wrong username or password"},
		{"auth failed", "authentication failed", "wrong username or password"},
		{"unrecognised", "<h1>503 Service Unavailable</h1>", "unexpected portal response (HTTP 503)"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := rejectionError([]byte(c.body), 503, "")
			if !strings.Contains(err.Error(), c.wantSubstr) {
				t.Errorf("rejectionError() = %q, want it to contain %q", err, c.wantSubstr)
			}

			path := dumpPath(t, err)
			saved, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("dump file %s unreadable: %v", path, readErr)
			}
			defer os.Remove(path)

			if string(saved) != c.body {
				t.Errorf("dump file = %q, want %q", saved, c.body)
			}
		})
	}
}

// The dump gets attached to bug reports, so a portal page that echoes the
// submitted password back must not carry it into the file.
func TestRejectionErrorRedactsPassword(t *testing.T) {
	body := `<input name="password" value="hunter2"><p>Invalid password</p>`

	err := rejectionError([]byte(body), 200, "hunter2")

	path := dumpPath(t, err)
	defer os.Remove(path)

	saved, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("dump file %s unreadable: %v", path, readErr)
	}
	if strings.Contains(string(saved), "hunter2") {
		t.Errorf("dump file leaked the password: %s", saved)
	}
	if !strings.Contains(string(saved), "[REDACTED]") {
		t.Errorf("dump file = %q, want the password replaced with [REDACTED]", saved)
	}
}

// dumpPath pulls the "saved to <path>" filename back out of the error message.
func dumpPath(t *testing.T, err error) string {
	t.Helper()

	m := regexp.MustCompile(`saved to (\S+)`).FindStringSubmatch(err.Error())
	if m == nil {
		t.Fatalf("error names no dump file: %v", err)
	}

	return m[1]
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
			// What the real BITS portal does: answers the probe with a page
			// pointing at /fgtauth?<magic> instead of setting a Location header.
			name: "intercept body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`<meta http-equiv="refresh" content="0; url=https://fw.bits-pilani.ac.in:8090/fgtauth?deadbeef12345678">`))
			},
			wantToken: "deadbeef12345678",
			wantWhich: "body",
			wantOk:    true,
		},
		{
			name: "form served inline in the probe body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`<input type="hidden" name="magic" value="deadbeef12345678">`))
			},
			wantToken: "deadbeef12345678",
			wantWhich: "body-form",
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
