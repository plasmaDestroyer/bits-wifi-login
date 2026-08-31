package installer

import (
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These run on every platform, which is the point: the launchd agent is the one
// trigger set nobody here can exercise by hand, so the generator has to be
// checked by CI rather than by owning a Mac.
func TestPlist(t *testing.T) {
	got := plist("/Users/someone/bin/bits-wifi-login")

	// Each of these is a trigger that silently does nothing if it goes missing,
	// and macOS gives no feedback when it does.
	for _, want := range []string{
		"<string>ac.bits.wifi-login</string>",                 // Label, or launchctl cannot address it
		"<string>/Users/someone/bin/bits-wifi-login</string>", // absolute path: a trigger has no working directory
		"<key>WatchPaths</key>",                               // fires on network change
		"<string>/var/run/resolv.conf</string>",               // ...this one specifically
		"<key>StartInterval</key>",                            // the only thing that notices an expiry
		"<integer>300</integer>",                              // 5 min, not Linux's 10 — no connectivity-change here
		"<key>RunAtLoad</key>",                                // fires at login
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plist() is missing %q\n%s", want, got)
		}
	}
}

// A path with an ampersand or a quote in it produces invalid XML unless it is
// escaped, and launchd rejects the whole file rather than the one key.
func TestPlistEscapesThePath(t *testing.T) {
	got := plist(`/Users/a&b/"quoted"/bits-wifi-login`)

	if strings.Contains(got, `&b`) && !strings.Contains(got, "&amp;b") {
		t.Errorf("plist() left a bare ampersand in the path:\n%s", got)
	}

	var parsed any
	if err := xml.Unmarshal([]byte(got), &parsed); err != nil {
		t.Errorf("plist() is not well-formed XML: %v\n%s", err, got)
	}
}

// Well-formed XML is not the same as a valid property list. plutil is what
// launchd itself uses, so this is the closest thing to "would macOS accept it"
// that exists — and it runs free on the macos-latest CI runner.
func TestPlistIsWellFormed(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("plutil only exists on macOS; the CI matrix covers this")
	}

	path := filepath.Join(t.TempDir(), "test.plist")
	if err := os.WriteFile(path, []byte(plist("/usr/local/bin/bits-wifi-login")), 0600); err != nil {
		t.Fatal(err)
	}

	if out, err := exec.Command("plutil", "-lint", path).CombinedOutput(); err != nil {
		t.Errorf("plutil rejected the generated plist: %v\n%s", err, out)
	}
}
