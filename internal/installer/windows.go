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
// need elevation at all — checking would only reject cases that work.
//
// The path is still checked. It no longer goes through a cmd.exe command line,
// only into an XML <Command> element, so a quote is merely impossible rather
// than dangerous — but a newline there would be folded into whitespace and the
// task would silently run something else. Windows filenames can hold neither,
// which makes this an assertion rather than a filter; assert it anyway.
func preflight(exe string) error {
	if strings.ContainsAny(exe, "\"\n\r") {
		return fmt.Errorf("installer: refusing to install from %q — the path must not contain quotes or newlines", exe)
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

	// S4U is what keeps the login invisible, but it needs the "log on as a batch
	// job" right, which not every account has. Degrading to InteractiveToken is
	// better than refusing to install — it only costs the window back — so try
	// the quiet one first and say plainly when we could not have it.
	logon := logonS4U

	for _, t := range tasks {
		out, err := createTask(t.name, t.xml(logon))
		if err != nil && logon == logonS4U {
			fmt.Printf("⚠ Windows would not register a background task: %s\n"+
				"  Falling back to an interactive task — a console window may flash on each run.\n",
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

	// schtasks /xml wants a Unicode file; see utf16LE.
	if err := os.WriteFile(path, utf16LE(doc), 0600); err != nil {
		return "", fmt.Errorf("installer: writing %s: %w", path, err)
	}
	defer os.Remove(path)

	return runOut("schtasks", "/create", "/tn", name, "/xml", path, "/f")
}

// firstLine keeps a schtasks complaint to the one line that says something.
func firstLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}

	return "no reason given"
}

// Ask before deleting, rather than deleting and interpreting the complaint.
// Deleting a task that was never registered makes schtasks print "ERROR: The
// system cannot find the file specified." straight to stderr — which is not an
// error worth showing at all, let alone next to our own line saying the same
// thing in kinder words. Reading that message back would also mean matching
// English text on a machine that may not be running in English, and would still
// leave a genuine refusal (access denied, a corrupt task) reported as "not
// found". A query settles it in the one way that is both quiet and honest.
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
		"    Get-Content \"" + runlog.Path() + "\" -Tail 50\n\n" +
		"  Repair:\n" +
		"    Re-run `bits-wifi-login install` if triggers or permissions break.\n"
}
