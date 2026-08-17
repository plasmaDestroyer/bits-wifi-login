// Package installer registers and removes the OS-native background triggers
// that run the login binary. Everything platform-specific lives in the
// build-tagged files beside this one; the shared preflight is here.
package installer

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/plasmaDestroyer/bits-wifi-login/internal/creds"
)

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

	fmt.Print("\n✓ Installation complete.\n\n" + summary())

	return nil
}

// Uninstall removes the triggers. creds.conf is deliberately left behind so a
// reinstall does not re-prompt.
func Uninstall() error {
	if err := uninstall(); err != nil {
		return err
	}

	fmt.Print("\n✓ Uninstall complete.\n\n" +
		"  creds.conf was left intact so you do not need to re-enter your\n" +
		"  credentials if you reinstall. Delete it manually if you are done.\n")

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
