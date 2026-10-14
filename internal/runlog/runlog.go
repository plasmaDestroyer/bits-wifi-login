// Package runlog keeps the run log on Windows, which has nowhere else to put
// it. Linux and macOS already have journald and the unified log.
package runlog

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Name is the log file's name.
const Name = "bits-wifi-login.log"

// maxBytes caps the live file before it rolls to <Name>.1: months of runs.
const maxBytes = 512 << 10

// Path is the log beside the binary, or "" on platforms that keep no file.
func Path() string {
	if runtime.GOOS != "windows" {
		return ""
	}

	exe, err := os.Executable()
	if err != nil {
		return Name
	}

	return filepath.Join(filepath.Dir(exe), Name)
}

// Open returns the log for appending, or nil. An unwritable log never fails a login.
func Open() *os.File {
	path := Path()
	if path == "" {
		return nil
	}

	return openAt(path)
}

// LastLine returns the newest log line for `status`, or "".
func LastLine() string {
	path := Path()
	if path == "" {
		return ""
	}

	return lastLineAt(path)
}

// lastLineAt reads only the tail of the file.
func lastLineAt(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return ""
	}

	const tail = 4 << 10

	at := info.Size() - tail
	if at < 0 {
		at = 0
	}

	buf := make([]byte, info.Size()-at)
	if _, err := f.ReadAt(buf, at); err != nil && err != io.EOF {
		return ""
	}

	lines := strings.Split(string(buf), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimRight(lines[i], "\r"); strings.TrimSpace(line) != "" {
			return line
		}
	}

	return ""
}

func openAt(path string) *os.File {
	rotate(path, maxBytes)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil
	}

	return f
}

// rotate keeps one old generation. Failures are silent: an oversized log beats
// a skipped login.
func rotate(path string, max int64) {
	info, err := os.Stat(path)
	if err != nil || info.Size() <= max {
		return
	}

	// os.Rename replaces the destination on Windows too.
	_ = os.Rename(path, path+".1")
}
