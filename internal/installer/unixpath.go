//go:build !windows

package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// ~/.local/bin symlink, not a shell rc edit: no guessing bash/zsh/fish, and
// trivial to remove. macOS lacks it on PATH, hence the warning in Install.
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

	// Replace only a symlink or nothing. Never someone's real file.
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

// unlink removes the symlink only if it still points at this binary.
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

func pathNote() string {
	return ""
}

func onPath(dir string) bool {
	return slices.Contains(filepath.SplitList(os.Getenv("PATH")), dir)
}
