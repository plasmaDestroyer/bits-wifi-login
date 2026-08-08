//go:build linux

package installer

import (
	"strings"
	"testing"
)

// The dispatcher runs as root and interpolates this path into a `su -c` string.
func TestUnsafePath(t *testing.T) {
	cases := []struct {
		path   string
		unsafe bool
	}{
		{"/home/user/.local/share/bits-wifi-login/bits-wifi-login", false},
		{"/opt/bits-wifi-login", false},
		{"/home/user/my dir/bits-wifi-login", true},   // systemd splits ExecStart on space
		{"/home/user/$(id)/bits-wifi-login", true},    // command substitution, as root
		{"/home/user/`id`/bits-wifi-login", true},     // ditto
		{`/home/user/"quoted"/bits-wifi-login`, true}, // breaks out of su -c "..."
		{"/home/user/a;rm -rf/bits-wifi-login", true},
		{"/home/user/a&b/bits-wifi-login", true},
	}

	for _, c := range cases {
		if got := unsafePath.MatchString(c.path); got != c.unsafe {
			t.Errorf("unsafePath(%q) = %v, want %v", c.path, got, c.unsafe)
		}
	}
}

func TestDispatcherUsesTheUserStateDir(t *testing.T) {
	got := dispatcher(
		"/opt/bits-wifi-login",
		"alice",
		"/home/alice/.local/state/bits-wifi-login/dispatcher.log",
	)

	if strings.Contains(got, "/tmp/") {
		t.Errorf("dispatcher writes into /tmp, which is symlink-attackable:\n%s", got)
	}
	if !strings.Contains(got, `su -c "/opt/bits-wifi-login >> /home/alice/.local/state/bits-wifi-login/dispatcher.log 2>&1" alice`) {
		t.Errorf("dispatcher su line is malformed:\n%s", got)
	}
}
