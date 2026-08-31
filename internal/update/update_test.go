package update

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func serve(t *testing.T, h http.HandlerFunc) {
	t.Helper()

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	restore := baseURL
	baseURL = srv.URL
	t.Cleanup(func() { baseURL = restore })
}

// The tag comes out of a redirect rather than the JSON API, so the thing that
// can break is the redirect shape, not any parsing of a body.
func TestLatest(t *testing.T) {
	cases := []struct {
		name     string
		location string
		status   int
		want     string
		wantErr  bool
	}{
		{"normal redirect", "https://github.com/x/y/releases/tag/v0.6.0", http.StatusFound, "v0.6.0", false},
		{"relative redirect", "/x/y/releases/tag/v1.2.3", http.StatusFound, "v1.2.3", false},
		// A 200 with no Location means GitHub answered something other than the
		// redirect — an error page, a login wall, a captive portal.
		{"no redirect at all", "", http.StatusOK, "", true},
		// Guards against treating "releases" or an HTML filename as a version and
		// then requesting a download URL built from it.
		{"not a tag", "https://github.com/x/y/releases", http.StatusFound, "", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			serve(t, func(w http.ResponseWriter, r *http.Request) {
				if c.location != "" {
					w.Header().Set("Location", c.location)
				}
				w.WriteHeader(c.status)
			})

			got, err := Latest()
			if (err != nil) != c.wantErr {
				t.Fatalf("Latest() error = %v, wantErr %v", err, c.wantErr)
			}
			if got != c.want {
				t.Errorf("Latest() = %q, want %q", got, c.want)
			}
		})
	}
}

// The asset name is a contract with release.yml and both bootstrap scripts.
func TestAsset(t *testing.T) {
	got := Asset()

	if !strings.HasPrefix(got, "bits-wifi-login-"+runtime.GOOS+"-"+runtime.GOARCH) {
		t.Errorf("Asset() = %q, want it to name this platform", got)
	}
	if (runtime.GOOS == "windows") != strings.HasSuffix(got, ".exe") {
		t.Errorf("Asset() = %q, wrong .exe suffix for %s", got, runtime.GOOS)
	}
}

// The dangerous failure: GitHub answers 200 with an error page and we write it
// over the running binary, leaving a scheduled task pointing at garbage.
func TestToRefusesATruncatedDownload(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<html>rate limited</html>")
	})

	err := To("v9.9.9")

	if err == nil || !strings.Contains(err.Error(), "refusing to install") {
		t.Fatalf("To() = %v, want a refusal on a too-small download", err)
	}
}

func TestToRejectsANonOK(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if err := To("v9.9.9"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("To() = %v, want the HTTP status reported", err)
	}
}

// Once a day, not once a trigger — the triggers fire every few minutes and none
// of them is a reason to ask GitHub anything.
func TestDue(t *testing.T) {
	stamp := filepath.Join(t.TempDir(), ".update-check")

	if !Due(stamp) {
		t.Fatal("Due() = false with no stamp file, want true")
	}
	if Due(stamp) {
		t.Error("Due() = true immediately after a check, want false")
	}

	old := time.Now().Add(-checkEvery - time.Minute)
	if err := os.Chtimes(stamp, old, old); err != nil {
		t.Fatal(err)
	}
	if !Due(stamp) {
		t.Error("Due() = false for a stale stamp, want true")
	}
}

// A missing stamp directory must not stop the check, only make it happen again
// sooner than intended.
func TestDueSurvivesAnUnwritableStamp(t *testing.T) {
	if !Due(filepath.Join(t.TempDir(), "no-such-dir", ".update-check")) {
		t.Error("Due() = false when the stamp cannot be written, want true")
	}
}

// A running binary must be replaceable in place, since the triggers bake its
// absolute path. On Unix that is one rename; on Windows the old file is moved
// aside first.
func TestReplace(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "bits-wifi-login")
	fresh := filepath.Join(dir, ".new")

	if err := os.WriteFile(exe, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fresh, []byte("new"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := replace(fresh, exe); err != nil {
		t.Fatalf("replace() = %v", err)
	}

	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("binary contains %q after replace, want %q", got, "new")
	}
}
