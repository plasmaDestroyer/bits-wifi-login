// Package runlog keeps the append-only log of trigger runs that the Windows
// build has nowhere else to put.
//
// Linux runs under systemd and macOS under launchd, so on those platforms the
// OS already keeps every line the binary writes and a file beside the binary
// would only be a second, worse copy. Open reports that by returning nil, and
// the caller carries on logging to stderr alone.
package runlog

import (
	"os"
	"path/filepath"
	"runtime"
)

// Name is the log file's name. The Windows task action used to hard-code this
// string separately; keep it here so the installer and the binary cannot drift.
const Name = "bits-wifi-login.log"

// maxBytes caps the live file before it is rolled to <Name>.1. A quiet run is
// one line of about eighty bytes and the Windows triggers fire every five
// minutes, so this holds several months and costs at most 1 MiB on disk.
const maxBytes = 512 << 10

// Path resolves the log beside the running binary, the same way creds.conf is
// resolved — the triggers that run it have no useful working directory.
func Path() string {
	exe, err := os.Executable()
	if err != nil {
		return Name
	}

	return filepath.Join(filepath.Dir(exe), Name)
}

// Open returns the run log open for appending, or nil when this platform logs
// somewhere better. A log we cannot write is never a reason to fail a login, so
// every error here is answered with nil rather than an error the caller would
// only have to ignore.
func Open() *os.File {
	if runtime.GOOS != "windows" {
		return nil
	}

	return openAt(Path())
}

func openAt(path string) *os.File {
	rotate(path, maxBytes)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil
	}

	return f
}

// rotate rolls the log over to <path>.1 once it outgrows max, keeping exactly
// one generation. Failure is silent by design: the worst case is a log that
// grows past the cap, which is still better than a run that refuses to happen.
func rotate(path string, max int64) {
	info, err := os.Stat(path)
	if err != nil || info.Size() <= max {
		return
	}

	// os.Rename replaces an existing destination on Windows too (MoveFileEx with
	// MOVEFILE_REPLACE_EXISTING), so the previous generation needs no unlink.
	_ = os.Rename(path, path+".1")
}
