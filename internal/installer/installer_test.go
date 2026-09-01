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

// Everything an install leaves behind has to be nameable, because "delete
// creds.conf yourself" is not an instruction if nothing ever said where it is.
func TestFilesStartsWithTheBinary(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("no executable path on this platform: %v", err)
	}

	files := Files()
	if len(files) == 0 {
		t.Fatal("Files() is empty — the uninstall notice would name nothing at all")
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

// Every file the tool writes beside itself has to appear here, or uninstall
// tells the user to delete a set that is missing one and the state survives a
// removal they were told was complete. session.json arrived with the expiry
// watcher and was absent from this list for exactly that reason.
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

// The point of listing leftovers is that the list is true. A creds.conf that
// was never created must not be reported as something to go and delete.
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

// Under a pipe or in CI there is no terminal to animate, and a redirected
// install log must not collect thousands of carriage returns. spin returns nil
// there, and a nil spinner still has to print its result rather than panic.
func TestSpinnerWithoutATerminal(t *testing.T) {
	s := spin("working")

	if s != nil {
		t.Error("spin() animated with no terminal on stdout")
	}

	s.done("✓ done") // must not panic on the nil receiver
}

// The whole point is that it stops. A frame left running would keep writing over
// whatever the install printed next.
func TestSpinnerStops(t *testing.T) {
	s := &spinner{stop: make(chan struct{}), finished: make(chan struct{})}
	go func() { <-s.stop; close(s.finished) }()

	done := make(chan struct{})
	go func() { s.done("✓ stopped"); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("done() did not return; the animation goroutine was never joined")
	}
}

// creds.conf must never exist in a half-written state: ensureCreds skips the
// prompt whenever the file is present, so a truncated one is silently accepted
// on every later run and the user is never asked again.
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

	// The mode carries the secret, so it has to survive the rename.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Errorf("creds.conf mode = %v, want 0600", info.Mode().Perm())
	}

	// No temp file may be left behind on the happy path.
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

// A failure must leave nothing behind, or the next run skips the prompt.
func TestWriteCredsLeavesNothingOnFailure(t *testing.T) {
	dir := t.TempDir()

	// A path inside a directory that does not exist: CreateTemp fails outright.
	err := writeCreds(filepath.Join(dir, "missing", "creds.conf"), creds.Creds{Username: "u", Password: "p"})
	if err == nil {
		t.Fatal("writeCreds() = nil for an unwritable path")
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("directory is not empty after a failed write: %v", entries)
	}
}
