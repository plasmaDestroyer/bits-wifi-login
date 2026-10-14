// Package installer registers and removes the OS-native background triggers
// that run the login binary. Platform bits live in the build-tagged files.
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

// Trigger is one background trigger and whether the OS still has it.
type Trigger struct {
	Name       string
	Registered bool
}

// Triggers asks the OS what is registered right now.
func Triggers() []Trigger {
	return triggers()
}

// Files lists what an install puts on disk, whether or not each path exists.
func Files() []string {
	files := []string{}

	if exe, err := os.Executable(); err == nil {
		files = append(files, exe)
	}

	// Not session.lock: it comes and goes, and would read "missing" forever.
	files = append(files, creds.DefaultPath(), session.DefaultPath())

	if path := runlog.Path(); path != "" {
		files = append(files, path)
	}

	return files
}

// Install prompts for credentials if needed, then wires up the triggers.
// Idempotent: re-running repairs a broken install.
func Install() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("installer: cannot locate the running binary: %w", err)
	}

	if err := preflight(exe); err != nil {
		return err
	}

	if err := ensureCreds(creds.DefaultPath()); err != nil {
		return err
	}

	if err := install(exe); err != nil {
		return err
	}

	// Not fatal: a working login is not worth failing over a PATH entry.
	linked, err := link(exe)
	if err != nil {
		fmt.Printf("⚠ Could not add %s to your PATH: %v\n"+
			"  The tool works regardless — run it by its full path.\n", filepath.Dir(exe), err)
	}

	fmt.Print("\n✓ Installation complete.\n\n" + summary() + "\n" + where(exe))

	if linked {
		fmt.Print(pathNote())
	}

	fmt.Print("\nAll set — enjoy never having to log in to the BITS portal by hand again.\n")

	return nil
}

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

// Uninstall removes the triggers. creds.conf stays so a reinstall won't re-prompt.
func Uninstall() error {
	removed, err := uninstall()
	if err != nil {
		return err
	}

	if exe, err := os.Executable(); err == nil && unlink(exe) {
		removed++
		fmt.Println("✓ Removed the PATH entry")
	}

	if removed == 0 {
		fmt.Print("\nNothing to remove — the background triggers were not registered.\n")
		return nil
	}

	fmt.Print("\n✓ Uninstall complete.\n\n" + leftovers())

	return nil
}

// leftovers names, with paths, what uninstall deliberately did not touch.
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

// writeCreds writes a 0600 temp file in the same directory (rename is only
// atomic on one filesystem) and renames it into place. A half-written
// creds.conf would skip the prompt forever after.
func writeCreds(path string, c creds.Creds) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".creds-*")
	if err != nil {
		return fmt.Errorf("installer: writing %s: %w", path, err)
	}
	defer os.Remove(tmp.Name())

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

	password, err := readMasked(int(os.Stdin.Fd()))
	if err != nil {
		return creds.Creds{}, fmt.Errorf("installer: reading password: %w — run this straight from a terminal, it cannot read a password through a pipe", err)
	}

	if username == "" || len(password) == 0 {
		return creds.Creds{}, errors.New("installer: username and password cannot be empty")
	}

	return creds.Creds{Username: username, Password: string(password)}, nil
}
