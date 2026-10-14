package portal

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
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
				case r.Method == http.MethodPost && r.URL.Path == "/": // no keepalive = rejected
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
		t.Error("keepalive endpoint never hit, so the session was not activated")
	}
	if s.LoginAt.IsZero() {
		t.Errorf("Login() session = %+v, want a login time", s)
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

// Seen live twice on 2026-08-31: a healed blip burned four attempts and exited fatally.
func TestLoginReportsAlreadyOnlineWhenTheProbeAnswers204(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	noFollow := srv.Client()
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	p := Portal{noFollow: noFollow, follow: srv.Client(), connectivityURL: srv.URL, baseURL: srv.URL}

	_, err := p.Login(creds.Creds{Username: "u", Password: "p"})

	if !errors.Is(err, ErrAlreadyOnline) {
		t.Errorf("Login() = %v, want ErrAlreadyOnline", err)
	}
}

func TestProbeGivesUpOnItsOwnDeadline(t *testing.T) {
	blocked := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocked
	}))
	defer srv.Close()
	defer close(blocked)

	restore := probeTimeout
	probeTimeout = 150 * time.Millisecond
	defer func() { probeTimeout = restore }()

	p := Portal{noFollow: srv.Client(), connectivityURL: srv.URL, baseURL: srv.URL}

	start := time.Now()
	online := p.IsLoggedIn()
	elapsed := time.Since(start)

	if online {
		t.Error("IsLoggedIn() = true against a server that never answers")
	}
	if elapsed > time.Second {
		t.Errorf("IsLoggedIn() took %v, want it bounded by probeTimeout", elapsed)
	}
}

// 2026-10-13: a hung probe inherited the last 204 and called a real expiry "came back on its own".
func TestLoginIgnoresAStale204AfterAFailedProbe(t *testing.T) {
	blocked := make(chan struct{})
	var sick atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !sick.Load() {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		<-blocked
	}))
	defer srv.Close()
	defer close(blocked)

	restore := probeTimeout
	probeTimeout = 50 * time.Millisecond
	defer func() { probeTimeout = restore }()

	p := Portal{noFollow: srv.Client(), follow: srv.Client(), connectivityURL: srv.URL, baseURL: srv.URL}
	if !p.IsLoggedIn() {
		t.Fatal("IsLoggedIn() = false against a 204")
	}

	sick.Store(true)
	p.IsLoggedIn() // the miss that sends the watcher into Login

	if _, err := p.Login(creds.Creds{Username: "u", Password: "p"}); errors.Is(err, ErrAlreadyOnline) {
		t.Error("Login() = ErrAlreadyOnline after a probe that never answered")
	}
}
