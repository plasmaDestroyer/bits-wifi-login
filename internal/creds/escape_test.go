package creds

import "testing"

// The whole point of Escape living next to unescape: whatever the installer
// writes, the loader must read back byte for byte.
func TestEscapeParseRoundTrip(t *testing.T) {
	values := []string{
		"plain",
		`with"quote`,
		`with\backslash`,
		"with\ttab",
		"with\nnewline",
		"with\rcarriage",
		`f2020123 \ " weird`,
		"  leading and trailing  ",
		"=equals=inside=",
		"'single quoted'",
		"",
	}

	for _, v := range values {
		t.Run(v, func(t *testing.T) {
			// An empty password is rejected by Parse on purpose, so pair each
			// value with a known-good counterpart and check one direction at a time.
			got, err := Parse(Format(Creds{Username: v, Password: "sentinel"}))
			if v == "" {
				if err == nil {
					t.Error("Parse of an empty username = nil error, want a rejection")
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(Format(%q)) errored: %v", v, err)
			}
			if got.Username != v {
				t.Errorf("round trip of %q gave %q", v, got.Username)
			}
			if got.Password != "sentinel" {
				t.Errorf("password round trip gave %q", got.Password)
			}
		})
	}
}

func TestEscape(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"plain", `"plain"`},
		{`a"b`, `"a\"b"`},
		{`a\b`, `"a\\b"`},
		{"a\tb", `"a\tb"`},
		{`a\"b`, `"a\\\"b"`}, // backslash must be escaped before the quote
	}

	for _, c := range cases {
		if got := Escape(c.in); got != c.want {
			t.Errorf("Escape(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}
