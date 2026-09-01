package installer

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

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

// The silent prompt looks like a frozen terminal to someone who has not met one,
// which is a Windows and macOS problem — a Linux user installing a CLI has.
func TestHiddenNote(t *testing.T) {
	got := hiddenNote()

	if runtime.GOOS == "linux" {
		if got != "" {
			t.Errorf("hiddenNote() = %q on linux, want it silent", got)
		}

		return
	}

	if !strings.Contains(got, "hidden") {
		t.Errorf("hiddenNote() = %q on %s, want it to explain the silent prompt", got, runtime.GOOS)
	}
}
