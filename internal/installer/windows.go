//go:build windows

package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/plasmaDestroyer/bits-wifi-login/internal/runlog"
)

// ponytail: no upfront elevation check. schtasks reports "Access is denied."
// clearly enough, and registering a task for the current user often does not
// need elevation at all, so checking would only reject cases that work.
//
// A newline in the path would be folded into XML whitespace and run something
// else. Windows filenames can't hold one. Assert it anyway.
func preflight(exe string) error {
	if strings.ContainsAny(exe, "\"\n\r") {
		return fmt.Errorf("installer: refusing to install from %q: the path must not contain quotes or newlines", exe)
	}

	return nil
}

func install(exe string) error {
	user := os.Getenv("USERDOMAIN") + `\` + os.Getenv("USERNAME")

	tasks := []struct {
		name string
		xml  func(logon string) string
		desc string
	}{
		{mainTask, func(l string) string { return mainTaskXML(user, exe, l) }, "every 5 minutes and on login"},
		{eventTask, func(l string) string { return eventTaskXML(user, exe, l) }, "on network connect and on resume"},
	}

	// S4U needs the "log on as a batch job" right. Without it fall back to
	// InteractiveToken and its console window rather than refuse to install.
	logon := logonS4U

	for _, t := range tasks {
		out, err := createTask(t.name, t.xml(logon))
		if err != nil && logon == logonS4U {
			fmt.Printf("⚠ Windows would not register a background task: %s\n"+
				"  Falling back to an interactive task. A console window may flash on each run.\n",
				firstLine(out))

			logon = logonInteractive
			out, err = createTask(t.name, t.xml(logon))
		}
		if err != nil {
			return fmt.Errorf("%w\n%s", err, strings.TrimSpace(out))
		}

		fmt.Printf("✓ Scheduled task %s registered (%s).\n", t.name, t.desc)
	}

	return nil
}

func createTask(name, doc string) (string, error) {
	path := filepath.Join(os.TempDir(), name+".xml")

	if err := os.WriteFile(path, utf16LE(doc), 0600); err != nil {
		return "", fmt.Errorf("installer: writing %s: %w", path, err)
	}
	defer os.Remove(path)

	return runOut("schtasks", "/create", "/tn", name, "/xml", path, "/f")
}

func firstLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}

	return "no reason given"
}

// Query before deleting: deleting a missing task spews an error, and parsing
// it would mean matching English text on a maybe-not-English machine.
func uninstall() (int, error) {
	removed := 0

	for _, name := range []string{mainTask, eventTask} {
		if _, err := runOut("schtasks", "/query", "/tn", name); err != nil {
			fmt.Printf("• Not registered: scheduled task %s\n", name)
			continue
		}

		if out, err := runOut("schtasks", "/delete", "/tn", name, "/f"); err != nil {
			return removed, fmt.Errorf("%w\n%s", err, strings.TrimSpace(out))
		}

		removed++
		fmt.Printf("✓ Removed scheduled task %s\n", name)
	}

	return removed, nil
}

func triggers() []Trigger {
	found := make([]Trigger, 0, 2)

	for _, name := range []string{mainTask, eventTask} {
		_, err := runOut("schtasks", "/query", "/tn", name)
		found = append(found, Trigger{Name: name, Registered: err == nil})
	}

	return found
}

func summary() string {
	return "  Triggers:\n" +
		"    - Every WiFi connect (NetworkProfile Event ID 10000)\n" +
		"    - Every resume from sleep (Power-Troubleshooter Event ID 1)\n" +
		"    - Every 5 minutes, and on login\n\n" +
		"  Logs:\n" +
		"    Get-Content \"" + runlog.Path() + "\" -Tail 50\n"
}
