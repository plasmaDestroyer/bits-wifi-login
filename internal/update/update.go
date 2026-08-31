// Package update replaces the running binary with the newest GitHub release.
//
// The triggers bake an absolute path to this binary, so an update has to be an
// in-place replacement of that exact file — anything else and the OS keeps
// running the old one forever.
package update

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	repo = "plasmaDestroyer/bits-wifi-login"
	// Checked at most this often on the automatic path. The triggers fire every
	// few minutes and none of them is a reason to ask GitHub anything.
	checkEvery = 24 * time.Hour
	// A release binary is ~10MB. Anything far smaller is an error page that
	// answered 200, and writing it over ourselves would be unrecoverable.
	minSize = 1 << 20
	timeout = 60 * time.Second
)

// A var so tests can point it at a local server. Nothing else should write it.
var baseURL = "https://github.com/" + repo

// Latest asks which release is newest by reading the redirect on
// /releases/latest, not the JSON API: the API allows 60 unauthenticated calls
// per hour per IP, and every machine on a campus NAT shares one IP.
func Latest() (string, error) {
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	res, err := client.Head(baseURL + "/releases/latest")
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	location := res.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("update: no redirect from /releases/latest (HTTP %d)", res.StatusCode)
	}

	tag := path.Base(location)
	if !strings.HasPrefix(tag, "v") {
		return "", fmt.Errorf("update: %q does not look like a release tag", tag)
	}

	return tag, nil
}

// Asset is the release file for this platform. The name is a contract with
// release.yml and the two bootstrap scripts — change it in one, change it in all.
func Asset() string {
	name := fmt.Sprintf("bits-wifi-login-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	return name
}

// To downloads tag and replaces the running binary with it.
func To(tag string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe) // never write over the ~/.local/bin link
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/releases/download/%s/%s", baseURL, tag, Asset())

	client := &http.Client{Timeout: timeout}
	res, err := client.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("update: %s returned HTTP %d", url, res.StatusCode)
	}

	// Download beside the target, never to a temp dir: a rename across
	// filesystems is not atomic, and this one has to be.
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".bits-wifi-login-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once the rename has taken it

	n, err := io.Copy(tmp, res.Body)
	if err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if n < minSize {
		return fmt.Errorf("update: %s was only %d bytes, refusing to install it", Asset(), n)
	}
	if err := os.Chmod(tmp.Name(), 0755); err != nil {
		return err
	}

	return replace(tmp.Name(), exe)
}

// Due reports whether enough time has passed since the last check, and records
// that one is happening now. Failing to write the stamp is not a reason to skip
// the update, only a reason to check again sooner than intended.
func Due(stamp string) bool {
	if info, err := os.Stat(stamp); err == nil && time.Since(info.ModTime()) < checkEvery {
		return false
	}

	now := time.Now()
	if err := os.WriteFile(stamp, nil, 0600); err == nil {
		os.Chtimes(stamp, now, now)
	}

	return true
}

func StampPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ".update-check"
	}

	return filepath.Join(filepath.Dir(exe), ".update-check")
}

// ErrDevBuild keeps a locally built binary from being replaced by a release.
// Without it, `go build && ./bits-wifi-login` would silently overwrite whatever
// was just compiled with whatever is on GitHub.
var ErrDevBuild = errors.New("update: this is a local build, not a release")
