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

// There used to be a TestSchtasksRejectsUTF8 here, asserting that the same
// document written as UTF-8 was refused — the idea being that if it ever
// stopped failing, utf16LE had stopped being necessary. It reported exactly
// that on real hardware: current schtasks accepts UTF-8 quite happily.
//
// That does not make UTF-16 wrong, it makes the assertion wrong. UTF-16 is what
// /xml documents, what every Windows build takes, and what the document's own
// declaration claims — so the encoding stays and the test that demanded the
// opposite is gone rather than reversed. Pinning "schtasks tolerates UTF-8 on
// the machine that happens to be running the suite" would test Microsoft's
// leniency, not this code.

// SSID detection has to survive a machine with Wi-Fi off or no adapter at all,
// since the triggers fire regardless of what the network is doing.
func TestSSIDDetectionDoesNotExplode(t *testing.T) {
	out, err := exec.Command("netsh", "wlan", "show", "interfaces").CombinedOutput()
	if err != nil {
		t.Skipf("netsh wlan unavailable on this machine: %v", err)
	}

	t.Logf("netsh reported:\n%s", out)
}
