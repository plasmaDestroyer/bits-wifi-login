package creds

import "testing"

func TestUnescape(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"passthrough", "abc", "abc"},
		{"escaped quote", `a\"b`, `a"b`},
		{"unknown escape", `a\xb`, `axb`},
		{"trailing backslash", `abc\`, `abc\`},
		{"escaped backslash", `a\\b`, `a\b`},
		{"newline", `a\nb`, "a\nb"},
		{"carriage return", `a\rb`, "a\rb"},
		{"tab", `a\tb`, "a\tb"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := unescape(c.in); got != c.want {
				t.Errorf("unescape(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}

}
