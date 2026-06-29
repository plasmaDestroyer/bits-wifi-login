package portal

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
				client:          srv.Client(),
				connectivityURL: srv.URL,
			}

			if got := p.IsLoggedIn(); got != c.want {
				t.Errorf("IsLoggedIn() = %v, want %v (server status %d)", got, c.want, c.status)
			}
		})
	}
}
