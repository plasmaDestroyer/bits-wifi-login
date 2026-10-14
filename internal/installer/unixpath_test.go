//go:build !windows

package installer

import (
	"os"
	"path/filepath"
	"testing"
)

func home(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("HOME", dir)

	return filepath.Join(dir, ".local", "bin", "bits-wifi-login")
}

func TestLinkCreatesTheSymlink(t *testing.T) {
	link, exe := home(t), filepath.Join(t.TempDir(), "bits-wifi-login")

	linked, err := link2(t, exe)
	if err != nil {
		t.Fatal(err)
	}
	if !linked {
		t.Error("link() reported no change on a machine with no link")
	}

	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("no symlink at %s: %v", link, err)
	}
	if target != exe {
		t.Errorf("symlink points at %q, want %q", target, exe)
	}
}

func TestLinkIsIdempotent(t *testing.T) {
	home(t)
	exe := filepath.Join(t.TempDir(), "bits-wifi-login")

	if _, err := link2(t, exe); err != nil {
		t.Fatal(err)
	}

	linked, err := link2(t, exe)
	if err != nil {
		t.Fatal(err)
	}
	if linked {
		t.Error("link() reported a change when the link was already correct")
	}
}

func TestLinkRepointsAStaleLink(t *testing.T) {
	link := home(t)
	old := filepath.Join(t.TempDir(), "old", "bits-wifi-login")
	new := filepath.Join(t.TempDir(), "new", "bits-wifi-login")

	if _, err := link2(t, old); err != nil {
		t.Fatal(err)
	}
	if _, err := link2(t, new); err != nil {
		t.Fatal(err)
	}

	if target, _ := os.Readlink(link); target != new {
		t.Errorf("symlink points at %q, want the new binary %q", target, new)
	}
}

func TestLinkRefusesToClobberARealFile(t *testing.T) {
	link := home(t)

	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(link, []byte("someone else's binary"), 0755); err != nil {
		t.Fatal(err)
	}

	if _, err := link2(t, filepath.Join(t.TempDir(), "bits-wifi-login")); err == nil {
		t.Error("link() overwrote a real file")
	}

	body, err := os.ReadFile(link)
	if err != nil || string(body) != "someone else's binary" {
		t.Error("the file that was already there did not survive")
	}
}

func TestUnlinkRemovesOurLink(t *testing.T) {
	link := home(t)
	exe := filepath.Join(t.TempDir(), "bits-wifi-login")

	if _, err := link2(t, exe); err != nil {
		t.Fatal(err)
	}

	if !unlink(exe) {
		t.Error("unlink() reported nothing removed")
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Error("the symlink survived unlink()")
	}
}

func TestUnlinkLeavesSomeoneElsesLinkAlone(t *testing.T) {
	link := home(t)
	theirs := filepath.Join(t.TempDir(), "their-build")

	if _, err := link2(t, theirs); err != nil {
		t.Fatal(err)
	}

	if unlink(filepath.Join(t.TempDir(), "bits-wifi-login")) {
		t.Error("unlink() claimed to remove a link that was not ours")
	}
	if target, _ := os.Readlink(link); target != theirs {
		t.Errorf("symlink now points at %q, want %q", target, theirs)
	}
}

func TestUnlinkOnAMachineWithNoLink(t *testing.T) {
	home(t)

	if unlink(filepath.Join(t.TempDir(), "bits-wifi-login")) {
		t.Error("unlink() claimed to remove a link that was never there")
	}
}

// link2 hides link()'s PATH advice, which always fires under a scratch HOME.
func link2(t *testing.T, exe string) (bool, error) {
	t.Helper()

	stdout := os.Stdout
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return link(exe)
	}

	os.Stdout = devnull
	defer func() {
		os.Stdout = stdout
		devnull.Close()
	}()

	return link(exe)
}
