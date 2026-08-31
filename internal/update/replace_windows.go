//go:build windows

package update

import "os"

// Windows will not let a running .exe be overwritten, but it will let one be
// renamed. So move ourselves aside first and put the new binary in the freed
// name; the leftover is deleted on the next update, since it is still locked
// while this process lives.
//
// If the second rename fails, put the old name back — a scheduled task pointing
// at a path with nothing on it is the one outcome worse than not updating.
func replace(newFile, exe string) error {
	old := exe + ".old"

	os.Remove(old) // a previous update's leftover, now unlocked

	if err := os.Rename(exe, old); err != nil {
		return err
	}

	if err := os.Rename(newFile, exe); err != nil {
		os.Rename(old, exe)
		return err
	}

	return nil
}
