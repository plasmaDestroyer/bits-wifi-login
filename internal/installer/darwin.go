//go:build darwin

package installer

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The plist is XML, so a path containing & or < would produce a file plutil
// rejects. Escape rather than forbid — macOS paths legitimately contain spaces.
func esc(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))

	return b.String()
}

const label = "ac.bits.wifi-login"

func plistPath() string {
	return filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", label+".plist")
}

func service() string {
	return fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
}

func preflight(exe string) error {
	if os.Geteuid() == 0 {
		return errors.New("installer: do not run this with sudo — it installs a per-user LaunchAgent")
	}

	if strings.ContainsAny(exe, "\n\r") {
		return fmt.Errorf("installer: refusing to install from %q — the path must not contain newlines", exe)
	}

	return nil
}

func install(exe string) error {
	path := plistPath()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("installer: creating LaunchAgents dir: %w", err)
	}

	// A binary fetched over the network can carry a quarantine flag Gatekeeper
	// refuses to run. Failure here is fine — a locally built binary has none.
	_ = run("xattr", "-d", "com.apple.quarantine", exe)

	if err := write(path, plist(exe), 0644); err != nil {
		return err
	}
	if err := run("plutil", "-lint", path); err != nil {
		return err
	}
	fmt.Println("✓ launchd plist created.")

	// Bootout first so re-running replaces a stale registration instead of failing.
	_ = run("launchctl", "bootout", service())

	if err := run("launchctl", "bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), path); err != nil {
		return err
	}
	_ = run("launchctl", "enable", service())
	fmt.Println("✓ launchd agent loaded.")

	return nil
}

func uninstall() error {
	if err := run("launchctl", "bootout", service()); err != nil {
		fmt.Println("⚠ Not loaded or already unloaded: launchd agent")
	} else {
		fmt.Println("✓ Unloaded launchd agent")
	}

	path := plistPath()
	if err := os.Remove(path); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("installer: removing %s: %w", path, err)
		}
		fmt.Printf("⚠ Not found: %s\n", path)
	} else {
		fmt.Printf("✓ Removed %s\n", path)
	}

	return nil
}

func summary() string {
	return "  Triggers:\n" +
		"    - Network changes (WatchPaths on resolv.conf — fires on most Wi-Fi connects)\n" +
		"    - Every 2 minutes (StartInterval — the reliable safety net)\n" +
		"    - At login (RunAtLoad)\n\n" +
		"  Logs:\n" +
		"    log show --predicate 'process == \"bits-wifi-login\"' --last 1h\n\n" +
		"  Repair:\n" +
		"    Re-run `bits-wifi-login install` if triggers or permissions break.\n"
}

func plist(exe string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>

    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>

    <key>WatchPaths</key>
    <array>
        <string>/var/run/resolv.conf</string>
    </array>

    <key>StartInterval</key>
    <integer>120</integer>

    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>
`, label, esc(exe))
}
