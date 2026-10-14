package installer

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/plasmaDestroyer/bits-wifi-login/internal/creds"
	"github.com/plasmaDestroyer/bits-wifi-login/internal/runlog"
	"github.com/plasmaDestroyer/bits-wifi-login/internal/session"
)

func TestFilesStartsWithTheBinary(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("no executable path on this platform: %v", err)
	}

	files := Files()
	if len(files) == 0 {
		t.Fatal("Files() is empty, so the uninstall notice would name nothing")
	}
	if files[0] != exe {
		t.Errorf("Files()[0] = %q, want the running binary %q", files[0], exe)
	}

	for _, path := range files {
		if !filepath.IsAbs(path) {
			t.Errorf("Files() gave %q, which is not somewhere a user can go and look", path)
		}
		if filepath.Dir(path) != filepath.Dir(exe) {
			t.Errorf("Files() gave %q, which is not beside the binary", path)
		}
	}
}

func TestFilesNamesEverythingWrittenBesideTheBinary(t *testing.T) {
	files := Files()

	for _, want := range []string{creds.DefaultPath(), session.DefaultPath()} {
		if !slices.Contains(files, want) {
			t.Errorf("Files() does not name %q:\n  %s", want, strings.Join(files, "\n  "))
		}
	}

	if path := runlog.Path(); path != "" && !slices.Contains(files, path) {
		t.Errorf("Files() does not name the run log %q:\n  %s", path, strings.Join(files, "\n  "))
	}
}

func TestLeftoversOnlyNamesFilesThatExist(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("no executable path on this platform: %v", err)
	}

	got := leftovers()

	if !strings.Contains(got, exe) {
		t.Errorf("leftovers() did not name the binary, which is certainly there:\n%s", got)
	}

	for _, path := range Files() {
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if strings.Contains(got, path) {
			t.Errorf("leftovers() told the user to delete %q, which does not exist:\n%s", path, got)
		}
	}
}

func TestWhereSaysHowToRemoveIt(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "bits-wifi-login")

	got := where(exe)

	if !strings.Contains(got, "uninstall") {
		t.Errorf("the install summary never mentions uninstall:\n%s", got)
	}
	if !strings.Contains(got, filepath.Dir(exe)) {
		t.Errorf("the install summary never says which folder to delete:\n%s", got)
	}
}

func TestSpinnerWithoutATerminal(t *testing.T) {
	s := spin("working")

	if s != nil {
		t.Error("spin() animated with no terminal on stdout")
	}

	s.done("✓ done")
}

func TestSpinnerStops(t *testing.T) {
	s := &spinner{stop: make(chan struct{}), finished: make(chan struct{})}
	go func() { <-s.stop; close(s.finished) }()

	done := make(chan struct{})
	go func() { s.done("✓ stopped"); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("done() did not return, so the animation goroutine was never joined")
	}
}

func TestWriteCredsIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creds.conf")

	if err := writeCreds(path, creds.Creds{Username: "u", Password: "p"}); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "u") || !strings.Contains(string(got), "p") {
		t.Errorf("creds.conf = %q, want the credentials", got)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Errorf("creds.conf mode = %v, want 0600", info.Mode().Perm())
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory holds %v, want only creds.conf", names)
	}
}

func TestWriteCredsLeavesNothingOnFailure(t *testing.T) {
	dir := t.TempDir()

	err := writeCreds(filepath.Join(dir, "missing", "creds.conf"), creds.Creds{Username: "u", Password: "p"})
	if err == nil {
		t.Fatal("writeCreds() = nil for an unwritable path")
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("directory is not empty after a failed write: %v", entries)
	}
}
