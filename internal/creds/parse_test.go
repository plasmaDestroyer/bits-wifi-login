package creds

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    Creds
		wantErr bool
	}{
		{"happy path", "USERNAME=\"alice\"\nPASSWORD=\"secret\"", Creds{Username: "alice", Password: "secret"}, false},
		{"missing password", "USERNAME=\"x\"\nPASSWORD=", Creds{}, true},
		{"single quoted", "USERNAME='alice'\nPASSWORD='secret'", Creds{"alice", "secret"}, false},
		{"utf-8 bom", "\ufeffUSERNAME=\"x\"\nPASSWORD=\"y\"", Creds{"x", "y"}, false},
		{"bare value", "USERNAME=alice\nPASSWORD=secret", Creds{"alice", "secret"}, false},
		{"last key wins", "USERNAME=first\nUSERNAME=alice\nPASSWORD=secret", Creds{"alice", "secret"}, false},
		{"equals in value", "USERNAME=alice\nPASSWORD=a=b=c", Creds{"alice", "a=b=c"}, false},
		{"crlf line endings", "USERNAME=alice\r\nPASSWORD=secret", Creds{"alice", "secret"}, false},
		{"spaces inside quotes", "USERNAME=\"  alice  \"\nPASSWORD=\"secret\"", Creds{"  alice  ", "secret"}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Parse(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if got != c.want {
				t.Errorf("Parse(%q) = %+v, want %+v", c.in, got, c.want)
			}
		})
	}
}
