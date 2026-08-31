//go:build !windows

package update

import "os"

// On Unix a running executable can be renamed over: the kernel holds the old
// inode open for processes already running it, and the next exec picks up the
// new file. One atomic syscall, so there is no window where the path does not
// exist.
func replace(newFile, exe string) error {
	return os.Rename(newFile, exe)
}
