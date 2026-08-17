//go:build windows

package installer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The one thing no amount of string testing can prove: that schtasks actually
// accepts what we generate. Everything up to this point can be green while
// schtasks still rejects the file for a reason it will not explain.
//
// Opt-in, because it registers a real scheduled task:
//
//	set BITS_WIFI_TEST_SCHTASKS=1 && go test ./internal/installer/ -run Schtasks -v
//
// It uses a throwaway task name and always deletes it, so it cannot disturb a
// real install.
func TestSchtasksAcceptsGeneratedXML(t *testing.T) {
	if os.Getenv("BITS_WIFI_TEST_SCHTASKS") != "1" {
		t.Skip("set BITS_WIFI_TEST_SCHTASKS=1 to run the real schtasks round-trip")
	}

	const testTask = "BITS-WiFi-Login-SelfTest"

	user := os.Getenv("USERDOMAIN") + `\` + os.Getenv("USERNAME")
	exe := filepath.Join(t.TempDir(), "bits-wifi-login.exe")

	// Both logon types, because install() falls back from one to the other and a
	// fallback nobody has ever exercised is not a fallback. S4U is the one that
	// matters: if this machine refuses it, every run shows a console window.
	for _, logon := range []string{logonS4U, logonInteractive} {
		t.Run(logon, func(t *testing.T) {
			xmlPath := filepath.Join(t.TempDir(), "selftest.xml")
			if err := os.WriteFile(xmlPath, utf16LE(mainTaskXML(user, exe, logon)), 0600); err != nil {
				t.Fatal(err)
			}

			// Delete first in case a previous failed run left it behind, and again after.
			exec.Command("schtasks", "/delete", "/tn", testTask, "/f").Run()
			t.Cleanup(func() {
				exec.Command("schtasks", "/delete", "/tn", testTask, "/f").Run()
			})

			out, err := exec.Command("schtasks", "/create", "/tn", testTask, "/xml", xmlPath, "/f").CombinedOutput()
			if err != nil {
				if strings.Contains(string(out), "Access is denied") {
					t.Fatalf("schtasks needs an elevated prompt — re-run this from an Administrator terminal.\n%s", out)
				}
				t.Fatalf("schtasks rejected the %s task: %v\n%s", logon, err, out)
			}

			if out, err := exec.Command("schtasks", "/query", "/tn", testTask).CombinedOutput(); err != nil {
				t.Fatalf("task did not register despite schtasks reporting success: %v\n%s", err, out)
			}
		})
	}
}

// Proves the UTF-16 encoding specifically, by feeding schtasks the same document
// as UTF-8 and requiring it to fail. If this ever passes, utf16LE has stopped
// being necessary and the comment claiming it is should be corrected.
func TestSchtasksRejectsUTF8(t *testing.T) {
	if os.Getenv("BITS_WIFI_TEST_SCHTASKS") != "1" {
		t.Skip("set BITS_WIFI_TEST_SCHTASKS=1 to run the real schtasks round-trip")
	}

	const testTask = "BITS-WiFi-Login-SelfTest-UTF8"

	user := os.Getenv("USERDOMAIN") + `\` + os.Getenv("USERNAME")
	doc := mainTaskXML(user, filepath.Join(t.TempDir(), "x.exe"), logonS4U)

	xmlPath := filepath.Join(t.TempDir(), "utf8.xml")
	if err := os.WriteFile(xmlPath, []byte(doc), 0600); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		exec.Command("schtasks", "/delete", "/tn", testTask, "/f").Run()
	})

	if err := exec.Command("schtasks", "/create", "/tn", testTask, "/xml", xmlPath, "/f").Run(); err == nil {
		t.Error("schtasks accepted a UTF-8 task file — utf16LE may no longer be needed")
	}
}

// SSID detection has to survive a machine with Wi-Fi off or no adapter at all,
// since the triggers fire regardless of what the network is doing.
func TestSSIDDetectionDoesNotExplode(t *testing.T) {
	out, err := exec.Command("netsh", "wlan", "show", "interfaces").CombinedOutput()
	if err != nil {
		t.Skipf("netsh wlan unavailable on this machine: %v", err)
	}

	t.Logf("netsh reported:\n%s", out)
}
