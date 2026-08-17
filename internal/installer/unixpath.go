//go:build !windows

package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ~/.local/bin, not a shell rc file. Editing .bashrc means guessing which of
// bash, zsh and fish the user actually runs, and leaving a line behind that
// uninstall then has to find again in a file the user has since edited. A
// symlink is one inode: trivially placed, trivially removed, and every
// mainstream distro already puts this directory on PATH when it exists.
//
// macOS does not, which is what the warning in Install is for.
func linkPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".local", "bin", "bits-wifi-login")
}

func link(exe string) (bool, error) {
	path := linkPath()
	if path == "" {
		return false, fmt.Errorf("installer: cannot locate your home directory")
	}

	if target, err := os.Readlink(path); err == nil && target == exe {
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, fmt.Errorf("installer: creating %s: %w", filepath.Dir(path), err)
	}

	// Replace whatever is there, but only if it is one of ours or nothing at
	// all — clobbering an unrelated binary someone put here would be worse than
	// leaving the command unavailable.
	switch _, err := os.Lstat(path); {
	case err == nil:
		if _, err := os.Readlink(path); err != nil {
			return false, fmt.Errorf("installer: %s already exists and is not a symlink — remove it or rename this binary", path)
		}
		if err := os.Remove(path); err != nil {
			return false, fmt.Errorf("installer: replacing %s: %w", path, err)
		}
	case !os.IsNotExist(err):
		return false, fmt.Errorf("installer: %s: %w", path, err)
	}

	if err := os.Symlink(exe, path); err != nil {
		return false, fmt.Errorf("installer: linking %s: %w", path, err)
	}

	if !onPath(filepath.Dir(path)) {
		fmt.Printf("⚠ %s is not on your PATH. Add it to use `bits-wifi-login` by name:\n"+
			"    export PATH=\"%s:$PATH\"\n", filepath.Dir(path), filepath.Dir(path))
	}

	return true, nil
}

// unlink removes the symlink, and only if it still points at this binary. A
// link someone repointed at a different build is theirs, not ours to delete.
func unlink(exe string) bool {
	path := linkPath()
	if path == "" {
		return false
	}

	if target, err := os.Readlink(path); err != nil || target != exe {
		return false
	}

	return os.Remove(path) == nil
}

// Nothing to note here, unlike Windows: the symlink lands in a directory that
// is already on PATH, so the name resolves in the very shell that ran install.
func pathNote() string {
	return ""
}

func onPath(dir string) bool {
	for _, entry := range strings.Split(os.Getenv("PATH"), string(os.PathListSeparator)) {
		if entry == dir {
			return true
		}
	}

	return false
}
