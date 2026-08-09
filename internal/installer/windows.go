//go:build windows

package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ponytail: no upfront elevation check. schtasks reports "Access is denied."
// clearly enough, and registering a task for the current user often does not
// need elevation at all — checking would only reject cases that work.
//
// The path does get checked: it is embedded in a quoted `cmd.exe /c` string, so
// a quote inside it would break out of the quoting. Windows filenames cannot
// contain one, which makes this an assertion rather than a filter — but the
// cost of being wrong is command execution, so assert it anyway.
func preflight(exe string) error {
	if strings.ContainsAny(exe, "\"\n\r") {
		return fmt.Errorf("installer: refusing to install from %q — the path must not contain quotes or newlines", exe)
	}

	return nil
}

func install(exe string) error {
	user := os.Getenv("USERDOMAIN") + `\` + os.Getenv("USERNAME")
	logFile := filepath.Join(filepath.Dir(exe), "bits-wifi-login.log")

	tasks := []struct {
		name string
		xml  string
		desc string
	}{
		{mainTask, mainTaskXML(user, exe, logFile), "every 5 minutes and on login"},
		{eventTask, eventTaskXML(user, exe, logFile), "on network connect and on resume"},
	}

	for _, t := range tasks {
		path := filepath.Join(os.TempDir(), t.name+".xml")

		// schtasks /xml requires a Unicode file, not UTF-8.
		if err := os.WriteFile(path, utf16LE(t.xml), 0600); err != nil {
			return fmt.Errorf("installer: writing %s: %w", path, err)
		}

		err := run("schtasks", "/create", "/tn", t.name, "/xml", path, "/f")
		os.Remove(path)
		if err != nil {
			return err
		}

		fmt.Printf("✓ Scheduled task %s registered (%s).\n", t.name, t.desc)
	}

	return nil
}

func uninstall() error {
	for _, name := range []string{mainTask, eventTask} {
		if err := run("schtasks", "/delete", "/tn", name, "/f"); err != nil {
			fmt.Printf("⚠ Not found: scheduled task %s\n", name)
		} else {
			fmt.Printf("✓ Removed scheduled task %s\n", name)
		}
	}

	return nil
}

func summary() string {
	return "  Triggers:\n" +
		"    - Every WiFi connect (NetworkProfile Event ID 10000)\n" +
		"    - Every resume from sleep (Power-Troubleshooter Event ID 1)\n" +
		"    - Every 5 minutes, and on login\n\n" +
		"  Logs:\n" +
		"    Get-Content bits-wifi-login.log -Tail 50\n\n" +
		"  Repair:\n" +
		"    Re-run `bits-wifi-login install` if triggers or permissions break.\n"
}
