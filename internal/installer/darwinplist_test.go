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

func TestPlist(t *testing.T) {
	got := plist("/Users/someone/bin/bits-wifi-login")

	for _, want := range []string{
		"<string>ac.bits.wifi-login</string>",
		"<string>/Users/someone/bin/bits-wifi-login</string>",
		"<key>WatchPaths</key>",
		"<string>/var/run/resolv.conf</string>",
		"<key>StartInterval</key>",
		"<integer>300</integer>",
		"<key>RunAtLoad</key>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plist() is missing %q\n%s", want, got)
		}
	}
}

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

// plutil is the closest thing to "would launchd accept it". Runs on the macOS CI runner.
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
