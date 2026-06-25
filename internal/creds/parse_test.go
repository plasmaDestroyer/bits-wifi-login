package creds

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Creds
	}{
		{"happy path", "USERNAME=\"alice\"\nPASSWORD=\"secret\"", Creds{Username: "alice", Password: "secret"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Parse(c.in)
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if got != c.want {
				t.Errorf("Parse(%q) = %+v, want %+v", c.in, got, c.want)
			}
		})
	}
}
