//go:build linux

package installer

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

// Executes the generated script: string-matching it is what let the 204/302 bug ship.
func TestDispatcherLogsInWhenTheProbeIsIntercepted(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		// The case the hook exists for: Fortinet answers the probe itself.
		{"intercepted", http.StatusOK, `<a href="/fgtauth?034ea02187c4c8d7">`, true},
		{"online", http.StatusNoContent, "", true},
		{"redirected", http.StatusFound, "", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := exec.LookPath("curl"); err != nil {
				t.Skip("curl not installed")
			}

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
				w.Write([]byte(c.body))
			}))
			defer srv.Close()

			dir := t.TempDir()
			marker := filepath.Join(dir, "logged-in")

			stub(t, dir, "nmcli", "#!/bin/sh\necho yes:BITS-STAFF\n")
			stub(t, dir, "su", "#!/bin/sh\ntouch "+marker+"\n")

			script := filepath.Join(dir, "dispatcher")
			body := strings.ReplaceAll(
				dispatcher("/opt/bits-wifi-login", "alice", filepath.Join(dir, "d.log")),
				"http://connectivitycheck.gstatic.com/generate_204", srv.URL,
			)
			if err := os.WriteFile(script, []byte(body), 0755); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command("bash", script, "wlan0", "connectivity-change")
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("dispatcher exited %v:\n%s", err, out)
			}

			if _, err := os.Stat(marker); (err == nil) != c.want {
				t.Errorf("HTTP %d: logged in = %v, want %v", c.status, err == nil, c.want)
			}
		})
	}
}

func stub(t *testing.T, dir, name, body string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestNMConnectivityGivesUpOnItsOwnDeadline(t *testing.T) {
	dir := t.TempDir()
	stub(t, dir, "nmcli", "#!/usr/bin/env bash\nsleep 30\necho full\n")
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	start := time.Now()
	_, err := nmConnectivity(200 * time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("nmConnectivity() = nil error against a probe that never answers")
	}
	if elapsed > 2*time.Second {
		t.Errorf("nmConnectivity() took %v, want it bounded by the deadline", elapsed)
	}
}

func TestSummaryMatchesWhatIsActuallyEnabled(t *testing.T) {
	got := summary()
	on := connectivityCheckEnabled()

	if on != strings.Contains(got, "connectivity-change") {
		t.Errorf("summary() names connectivity-change = %v, but it is enabled = %v\n%s",
			!on, on, got)
	}

	if !on && !strings.Contains(got, "only drop detection") {
		t.Errorf("summary() does not say the timer is the only detection left:\n%s", got)
	}

	for _, want := range []string{"dispatcher", "resume", "systemd timer"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary() is missing %q:\n%s", want, got)
		}
	}
}

func TestConnectivityConfSetsResponseAlongsideURI(t *testing.T) {
	if !strings.Contains(connectivityConf, "uri=") {
		t.Fatal("connectivityConf no longer sets uri, so this test needs rewriting")
	}

	if !strings.Contains(connectivityConf, "\nresponse=\n") {
		t.Errorf("connectivityConf overrides uri without an empty response:\n%s", connectivityConf)
	}
}
