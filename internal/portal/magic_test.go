package portal

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFixtures runs the extractors against captured portal HTML rather than the
// hand-written snippets above — the snippets pin the regex, these pin the shapes
// the real portal has actually served.
func TestFixtures(t *testing.T) {
	cases := []struct {
		file      string
		extract   func(string) (string, bool)
		wantToken string
		wantOK    bool
	}{
		// Captured from the real BITS portal on 2026-08-08 (token replaced). This
		// is the page a browser actually lands on: the magic is only in the hidden
		// form input, so magicFromBody finds nothing and the form strategy carries it.
		{"portal_form_real.html", magicFromForm, "a1b2c3d4e5f60718", true},
		{"portal_form_real.html", magicFromBody, "", false},
		{"intercept_fgtauth.html", magicFromBody, "deadbeef12345678", true},
		{"intercept_magic.html", magicFromBody, "deadbeef12345678", true},
		{"portal_form.html", magicFromForm, "deadbeef12345678", true},
		{"login_success.html", keepaliveFromBody, "cafebabe87654321", true},
		{"login_failure.html", keepaliveFromBody, "", false},
	}

	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("testdata", c.file))
			if err != nil {
				t.Fatal(err)
			}

			got, ok := c.extract(string(body))
			if got != c.wantToken || ok != c.wantOK {
				t.Errorf("%s = (%q, %v), want (%q, %v)", c.file, got, ok, c.wantToken, c.wantOK)
			}
		})
	}
}

// The failure page must also be recognised as a credential rejection, not as an
// unexpected response.
func TestFixtureLoginFailureIsCredentialRejection(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "login_failure.html"))
	if err != nil {
		t.Fatal(err)
	}

	if !rejectRe.Match(body) {
		t.Error("login_failure.html not matched by rejectRe — would be reported as an unexpected response")
	}
}

func TestMagicFromRedirect(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantToken string
		wantOK    bool
	}{
		{"redirect url", "https://fw.bits-pilani.ac.in:8090/fgtauth?deadbeef12345678", "deadbeef12345678", true},
		{"equals separator", "https://fw.bits-pilani.ac.in:8090/fgtauth=deadbeef12345678", "deadbeef12345678", true},
		{"no token", "https://example.com/", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := magicFromRedirect(c.in)
			if got != c.wantToken || ok != c.wantOK {
				t.Errorf("magicFromRedirect(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.wantToken, c.wantOK)
			}
		})
	}
}

func TestMagicFromBody(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantToken string
		wantOK    bool
	}{
		{"intercept fgtauth", "url=https://fw.bits-pilani.ac.in:8090/fgtauth?deadbeef12345678", "deadbeef12345678", true},
		{"intercept magic", "magic=deadbeef12345678", "deadbeef12345678", true},
		{"no token", "https://example.com/", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := magicFromBody(c.in)
			if got != c.wantToken || ok != c.wantOK {
				t.Errorf("magicFromRedirect(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.wantToken, c.wantOK)
			}
		})
	}
}

func TestMagicFromForm(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantToken string
		wantOK    bool
	}{
		{"portal form", `name="magic" value="deadbeef12345678"`, "deadbeef12345678", true},
		{"no token", "https://example.com/", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := magicFromForm(c.in)
			if got != c.wantToken || ok != c.wantOK {
				t.Errorf("magicFromRedirect(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.wantToken, c.wantOK)
			}
		})
	}
}

func TestKeepaliveFromBody(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantToken string
		wantOK    bool
	}{
		{"login failure", `name="magic" value="deadbeef12345678"`, "", false},
		{"login success", `location.href="https://fw.bits-pilani.ac.in:8090/keepalive?cafebabe87654321";`, "cafebabe87654321", true},
		{"no token", "https://example.com/", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := keepaliveFromBody(c.in)
			if got != c.wantToken || ok != c.wantOK {
				t.Errorf("magicFromRedirect(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.wantToken, c.wantOK)
			}
		})
	}
}
