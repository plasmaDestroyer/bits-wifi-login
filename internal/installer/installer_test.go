package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
