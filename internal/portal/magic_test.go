package portal

import "testing"

func TestMagicFromRedirect(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantToken string
		wantOK    bool
	}{
		{"redirect url", "https://fw.bits-pilani.ac.in:8090/fgtauth?deadbeef12345678", "deadbeef12345678", true},
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
