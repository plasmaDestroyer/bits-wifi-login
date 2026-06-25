package creds

import "testing"

func TestUnescape(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"passthrough", "abc", "abc"},
		{"escaped backslash", `a\\b`, `a\b`},
		{"newline", `a\nb`, "a\nb"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := unescape(c.in); got != c.want {
				t.Errorf("unescape(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}

}
