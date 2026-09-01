// Package installer registers and removes the OS-native background triggers
// that run the login binary. Everything platform-specific lives in the
// build-tagged files beside this one; the shared preflight is here.
package installer

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/plasmaDestroyer/bits-wifi-login/internal/creds"
	"github.com/plasmaDestroyer/bits-wifi-login/internal/runlog"
	"github.com/plasmaDestroyer/bits-wifi-login/internal/session"
)

// Trigger is one background trigger and whether the OS still has it. Asking the
// OS is the only honest answer: an install can rot without anything telling the
// user, and "is it still set up?" should not require learning schtasks.
type Trigger struct {
	Name       string
	Registered bool
}

// Triggers reports the platform's triggers, newest install or not.
func Triggers() []Trigger {
	return triggers()
}

// Files lists what an install puts on disk: the binary, then whatever it keeps
// beside itself. Every path here is reported whether or not it exists — callers
// that only want the real ones say so.
func Files() []string {
	files := []string{}

	if exe, err := os.Executable(); err == nil {
		files = append(files, exe)
	}

	// session.json but not session.lock: the lock is machinery that creates and
	// removes itself, so it would read as "missing" in status forever and offer
	// nothing to delete. A stale one left by a killed watcher goes with the
	// directory, which is what the removal notice says to delete anyway.
	files = append(files, creds.DefaultPath(), session.DefaultPath())

	if path := runlog.Path(); path != "" {
		files = append(files, path)
	}

	return files
}

// Install prompts for credentials if needed, then wires up the platform's
// triggers. It is idempotent — re-running repairs a broken install.
func Install() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("installer: cannot locate the running binary: %w", err)
	}

	// The binary path is interpolated into a root-run dispatcher script, a
	// systemd unit and a cmd.exe command line. Reject anything those cannot
	// carry safely rather than trying to quote for three grammars at once.
	if err := preflight(exe); err != nil {
		return err
	}

	if err := ensureCreds(creds.DefaultPath()); err != nil {
		return err
	}

	if err := install(exe); err != nil {
		return err
	}

	// Every instruction this tool prints, and every line of the README, says to
	// run `bits-wifi-login <something>`. None of that is true while the install
	// directory is somewhere PATH has never heard of. Not fatal, though: a tool
	// that logs you in perfectly well is not worth failing over a PATH entry.
	linked, err := link(exe)
	if err != nil {
		fmt.Printf("⚠ Could not add %s to your PATH: %v\n"+
			"  The tool works regardless — run it by its full path.\n", filepath.Dir(exe), err)
	}

	fmt.Print("\n✓ Installation complete.\n\n" + summary() + "\n" + where(exe))

	// Only when this run is what put it on PATH. On a repair the user is already
	// standing in a terminal that works, and telling them to close it is wrong.
	if linked {
		fmt.Print(pathNote())
	}

	fmt.Print("\nAll set — enjoy never having to log in to the BITS portal by hand again.\n")

	return nil
}

// where answers the two questions the install output never used to: what did
// this put on my machine, and how do I get rid of it.
func where(exe string) string {
	b := &strings.Builder{}

	b.WriteString("  Files:\n")
	for _, path := range Files() {
		b.WriteString("    " + path + "\n")
	}

	b.WriteString("\n  Remove:\n" +
		"    `bits-wifi-login uninstall` takes out the background triggers and\n" +
		"    the PATH entry. The files above are just files — delete\n" +
		"    " + filepath.Dir(exe) + " when you are done with them.\n")

	return b.String()
}

// Uninstall removes the triggers. creds.conf is deliberately left behind so a
// reinstall does not re-prompt.
func Uninstall() error {
	removed, err := uninstall()
	if err != nil {
		return err
	}

	if exe, err := os.Executable(); err == nil && unlink(exe) {
		removed++
		fmt.Println("✓ Removed the PATH entry")
	}

	// "✓ Uninstall complete." after removing nothing at all reads as a job well
	// done, which is how running uninstall twice ends up looking like a bug.
	if removed == 0 {
		fmt.Print("\nNothing to remove — the background triggers were not registered.\n")
		return nil
	}

	fmt.Print("\n✓ Uninstall complete.\n\n" + leftovers())

	return nil
}

// leftovers names what uninstall deliberately did not touch. Saying "creds.conf
// was left intact" without saying where it is leaves the user to go looking for
// a file whose location was never printed anywhere.
func leftovers() string {
	b := &strings.Builder{}

	b.WriteString("  Your credentials were left intact so a reinstall does not re-prompt.\n" +
		"  Delete these yourself when you are done with the tool:\n")

	for _, path := range Files() {
		if _, err := os.Stat(path); err == nil {
			b.WriteString("    " + path + "\n")
		}
	}

	return b.String()
}

// writeCreds writes into a temporary file and renames it into place, so
// creds.conf either does not exist or is complete. os.WriteFile creates the file
// first and fills it after, so a process killed in between leaves a truncated
// creds.conf behind — and because ensureCreds skips the prompt whenever the file
// exists, that half-written file would be silently accepted on every later run.
//
// The temp file is created in the same directory, since a rename is only atomic
// within one filesystem. os.CreateTemp already makes it 0600, which is the mode
// creds.conf has to keep.
func writeCreds(path string, c creds.Creds) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".creds-*")
	if err != nil {
		return fmt.Errorf("installer: writing %s: %w", path, err)
	}
	defer os.Remove(tmp.Name()) // no-op once the rename has taken it

	if _, err := tmp.WriteString(creds.Format(c)); err != nil {
		tmp.Close()
		return fmt.Errorf("installer: writing %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("installer: writing %s: %w", path, err)
	}

	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("installer: writing %s: %w", path, err)
	}

	return nil
}

func ensureCreds(path string) error {
	if _, err := os.Stat(path); err == nil {
		fmt.Println("✓ creds.conf already exists, skipping.")
		return nil
	}

	fmt.Println("No creds.conf found. Let's create one.")

	c, err := prompt()
	if err != nil {
		return err
	}

	if err := writeCreds(path, c); err != nil {
		return err
	}

	fmt.Println("✓ creds.conf created.")

	return nil
}

func prompt() (creds.Creds, error) {
	fmt.Print("Enter your BITS username: ")

	username, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return creds.Creds{}, fmt.Errorf("installer: reading username: %w", err)
	}
	username = strings.TrimSpace(username)

	fmt.Print("Enter your BITS password: ")

	// Echoes a star per character, so the prompt cannot be mistaken for a frozen
	// terminal and no explanatory line is needed on any platform.
	password, err := readMasked(int(os.Stdin.Fd()))
	if err != nil {
		return creds.Creds{}, fmt.Errorf("installer: reading password: %w — run this straight from a terminal, it cannot read a password through a pipe", err)
	}

	if username == "" || len(password) == 0 {
		return creds.Creds{}, errors.New("installer: username and password cannot be empty")
	}

	return creds.Creds{Username: username, Password: string(password)}, nil
}
