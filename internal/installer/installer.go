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

	"golang.org/x/term"

	"github.com/plasmaDestroyer/bits-wifi-login/internal/creds"
	"github.com/plasmaDestroyer/bits-wifi-login/internal/runlog"
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

	files = append(files, creds.DefaultPath())

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
	switch linked, err := link(exe); {
	case err != nil:
		fmt.Printf("⚠ Could not add %s to your PATH: %v\n"+
			"  The tool works regardless — run it by its full path.\n", filepath.Dir(exe), err)
	case linked:
		fmt.Printf("✓ Added to your PATH — open a new terminal to use `bits-wifi-login` by name.\n")
	}

	fmt.Print("\n✓ Installation complete.\n\n" + summary() + "\n" + where(exe))

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

	if err := os.WriteFile(path, []byte(creds.Format(c)), 0600); err != nil {
		return fmt.Errorf("installer: writing %s: %w", path, err)
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

	// term.ReadPassword echoes nothing at all — not even asterisks, and the
	// cursor does not move — so say so. Otherwise the prompt is indistinguishable
	// from a frozen terminal and people retype, paste, or kill the installer.
	fmt.Print("Enter your BITS password (hidden — nothing appears as you type, press Enter when done): ")

	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return creds.Creds{}, fmt.Errorf("installer: reading password: %w — run this straight from a terminal, it cannot read a password through a pipe", err)
	}

	if username == "" || len(password) == 0 {
		return creds.Creds{}, errors.New("installer: username and password cannot be empty")
	}

	return creds.Creds{Username: username, Password: string(password)}, nil
}
