package portal

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The portal sets a session cookie on GET /fgtauth?<magic> (issued by follow) and
// expects it back on the credential POST (issued by noFollow). Separate jars would
// silently drop it and the login would be rejected.
func TestNewSharesCookieJar(t *testing.T) {
	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set":
			http.SetCookie(w, &http.Cookie{Name: "SVPNCOOKIE", Value: "session123", Path: "/"})
		case "/echo":
			if c, err := r.Cookie("SVPNCOOKIE"); err == nil {
				gotCookie = c.Value
			}
		}
	}))
	defer srv.Close()

	p := New()
	if _, err := p.follow.Get(srv.URL + "/set"); err != nil {
		t.Fatalf("follow.Get: %v", err)
	}
	if _, err := p.noFollow.Get(srv.URL + "/echo"); err != nil {
		t.Fatalf("noFollow.Get: %v", err)
	}

	if gotCookie != "session123" {
		t.Errorf("noFollow sent cookie %q, want %q — clients are not sharing a jar", gotCookie, "session123")
	}
}

func TestNewNoFollowStopsAtRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
		}
	}))
	defer srv.Close()

	res, err := New().noFollow.Get(srv.URL)
	if err != nil {
		t.Fatalf("noFollow.Get: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusFound {
		t.Errorf("noFollow status = %d, want %d — redirect was followed, Location header is lost",
			res.StatusCode, http.StatusFound)
	}
}

func TestIsLoggedIn(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   bool
	}{
		{"online 204", http.StatusNoContent, true},
		{"not online 200", http.StatusOK, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
			}))
			defer srv.Close()

			p := Portal{
				noFollow:        srv.Client(),
				connectivityURL: srv.URL,
			}

			if got := p.IsLoggedIn(); got != c.want {
				t.Errorf("IsLoggedIn() = %v, want %v (server status %d)", got, c.want, c.status)
			}
		})
	}
}
