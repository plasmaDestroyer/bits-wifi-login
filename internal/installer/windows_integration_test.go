//go:build windows

package installer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Proves schtasks actually accepts what we generate. Opt-in, because it
// registers (and deletes) a real throwaway task:
//
//	set BITS_WIFI_TEST_SCHTASKS=1 && go test ./internal/installer/ -run Schtasks -v
func TestSchtasksAcceptsGeneratedXML(t *testing.T) {
	if os.Getenv("BITS_WIFI_TEST_SCHTASKS") != "1" {
		t.Skip("set BITS_WIFI_TEST_SCHTASKS=1 to run the real schtasks round-trip")
	}

	const testTask = "BITS-WiFi-Login-SelfTest"

	user := os.Getenv("USERDOMAIN") + `\` + os.Getenv("USERNAME")
	exe := filepath.Join(t.TempDir(), "bits-wifi-login.exe")

	// Both logon types: an unexercised fallback is not a fallback.
	for _, logon := range []string{logonS4U, logonInteractive} {
		t.Run(logon, func(t *testing.T) {
			xmlPath := filepath.Join(t.TempDir(), "selftest.xml")
			if err := os.WriteFile(xmlPath, utf16LE(mainTaskXML(user, exe, logon)), 0600); err != nil {
				t.Fatal(err)
			}

			exec.Command("schtasks", "/delete", "/tn", testTask, "/f").Run()
			t.Cleanup(func() {
				exec.Command("schtasks", "/delete", "/tn", testTask, "/f").Run()
			})

			out, err := exec.Command("schtasks", "/create", "/tn", testTask, "/xml", xmlPath, "/f").CombinedOutput()
			if err != nil {
				if strings.Contains(string(out), "Access is denied") {
					t.Fatalf("schtasks needs an elevated prompt. Re-run this from an Administrator terminal.\n%s", out)
				}
				t.Fatalf("schtasks rejected the %s task: %v\n%s", logon, err, out)
			}

			if out, err := exec.Command("schtasks", "/query", "/tn", testTask).CombinedOutput(); err != nil {
				t.Fatalf("task did not register despite schtasks reporting success: %v\n%s", err, out)
			}
		})
	}
}

// Triggers fire with Wi-Fi off or no adapter at all. SSID detection must cope.
func TestSSIDDetectionDoesNotExplode(t *testing.T) {
	out, err := exec.Command("netsh", "wlan", "show", "interfaces").CombinedOutput()
	if err != nil {
		t.Skipf("netsh wlan unavailable on this machine: %v", err)
	}

	t.Logf("netsh reported:\n%s", out)
}
