package runlog

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOpenAppendsRatherThanTruncates(t *testing.T) {
	path := filepath.Join(t.TempDir(), Name)

	if err := os.WriteFile(path, []byte("first run\n"), 0600); err != nil {
		t.Fatal(err)
	}

	f := openAt(path)
	if f == nil {
		t.Fatal("openAt returned nil for a writable directory")
	}
	if _, err := f.WriteString("second run\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	got := read(t, path)
	if got != "first run\nsecond run\n" {
		t.Errorf("log = %q, want both runs", got)
	}
}

func TestOpenCreatesAMissingLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), Name)

	f := openAt(path)
	if f == nil {
		t.Fatal("openAt returned nil for a missing file it should have created")
	}
	f.Close()

	if _, err := os.Stat(path); err != nil {
		t.Errorf("log was not created: %v", err)
	}
}

// An unwritable log must degrade to "no log", never to a failed login.
func TestOpenReturnsNilRatherThanFailing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a missing parent directory is the portable way to force this")
	}

	if f := openAt(filepath.Join(t.TempDir(), "no-such-dir", Name)); f != nil {
		f.Close()
		t.Error("openAt succeeded against a missing parent directory")
	}
}

func TestRotateLeavesASmallLogAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), Name)
	if err := os.WriteFile(path, []byte("short\n"), 0600); err != nil {
		t.Fatal(err)
	}

	rotate(path, 1024)

	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Error("rotated a log that was under the cap")
	}
	if got := read(t, path); got != "short\n" {
		t.Errorf("log = %q, want it untouched", got)
	}
}

func TestRotateRollsAnOversizedLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, Name)

	old := strings.Repeat("x", 2048)
	if err := os.WriteFile(path, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}

	rotate(path, 1024)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("live log survived rotation — the next run will keep appending to it")
	}
	if got := read(t, path+".1"); got != old {
		t.Error(".1 does not hold what the live log held")
	}

	// The rolled log must not accumulate generations, only replace the one.
	if err := os.WriteFile(path, []byte(strings.Repeat("y", 2048)), 0600); err != nil {
		t.Fatal(err)
	}
	rotate(path, 1024)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("kept %d files, want only %s.1", len(entries), Name)
	}
}

// The file only exists on Windows; everywhere else journald and the unified log
// already have the output, and a second copy would just rot.
func TestOpenIsWindowsOnly(t *testing.T) {
	f := Open()
	if f != nil {
		f.Close()
	}

	if (f != nil) != (runtime.GOOS == "windows") {
		t.Errorf("Open() non-nil = %v on %s", f != nil, runtime.GOOS)
	}
}

func TestPathSitsBesideTheBinary(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("no executable path on this platform: %v", err)
	}

	if want := filepath.Join(filepath.Dir(exe), Name); Path() != want {
		t.Errorf("Path() = %q, want %q", Path(), want)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(b)
}
