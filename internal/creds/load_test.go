package creds

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "creds.conf")
	if err := os.WriteFile(path, []byte("USERNAME=\"alice\"\nPASSWORD=\"s\\tecret\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if want := (Creds{"alice", "s\tecret"}); got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}

	// Windows has no Unix mode bits — os.Chmod there only toggles the read-only
	// attribute, so the tightening Load does is a no-op and asserting it would
	// fail for the wrong reason. See the comment on Load.
	if runtime.GOOS == "windows" {
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.conf")); err == nil {
		t.Error("Load() on missing file = nil, want error")
	}
}
