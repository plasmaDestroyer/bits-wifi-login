//go:build windows

package installer

import (
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// HKCU\Environment is the per-user PATH: no elevation needed, and no way to
// damage the machine-wide one next to it.
//
// Not setx. setx would do this in a single line and silently truncate the
// value at 1024 characters on the way, which is the classic way to destroy
// somebody's PATH. Not the process environment either — that dies with us.
const envKey = `Environment`

func link(exe string) (bool, error) {
	dir := filepath.Dir(exe)

	key, err := registry.OpenKey(registry.CURRENT_USER, envKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false, fmt.Errorf("installer: opening HKCU\\%s: %w", envKey, err)
	}
	defer key.Close()

	current, kind, err := key.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return false, fmt.Errorf("installer: reading your PATH: %w", err)
	}

	if listed(current, dir) {
		return false, nil
	}

	updated := dir
	if current = strings.TrimRight(current, ";"); current != "" {
		updated = current + ";" + dir
	}

	// Preserve the type. A user PATH is normally REG_EXPAND_SZ and often holds
	// %USERPROFILE%; rewriting it as a plain string would freeze those.
	if kind == registry.SZ {
		err = key.SetStringValue("Path", updated)
	} else {
		err = key.SetExpandStringValue("Path", updated)
	}
	if err != nil {
		return false, fmt.Errorf("installer: writing your PATH: %w", err)
	}

	announce()

	return true, nil
}

func unlink(exe string) bool {
	dir := filepath.Dir(exe)

	key, err := registry.OpenKey(registry.CURRENT_USER, envKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	current, kind, err := key.GetStringValue("Path")
	if err != nil {
		return false
	}

	kept := make([]string, 0, strings.Count(current, ";")+1)
	for _, entry := range strings.Split(current, ";") {
		if entry != "" && !samePath(entry, dir) {
			kept = append(kept, entry)
		}
	}

	updated := strings.Join(kept, ";")
	if updated == current {
		return false
	}

	if kind == registry.SZ {
		err = key.SetStringValue("Path", updated)
	} else {
		err = key.SetExpandStringValue("Path", updated)
	}
	if err != nil {
		return false
	}

	announce()

	return true
}

func listed(path, dir string) bool {
	for _, entry := range strings.Split(path, ";") {
		if samePath(entry, dir) {
			return true
		}
	}

	return false
}

// Windows paths are case-insensitive, and a PATH entry may or may not carry a
// trailing separator or be wrapped in quotes.
func samePath(a, b string) bool {
	clean := func(s string) string {
		s = strings.TrimSpace(s)
		s = strings.Trim(s, `"`)

		return strings.ToLower(strings.TrimRight(s, `\/`))
	}

	return clean(a) != "" && clean(a) == clean(b)
}

// announce tells the desktop the environment changed. Without this the new PATH
// only reaches processes started after the next sign-in: Explorer caches the
// environment it hands to everything it launches, so even a freshly opened
// terminal would inherit the old one. With it, opening a new terminal is enough.
//
// Best effort — the PATH is already written, and a broadcast that fails costs a
// sign-in, not the install.
func announce() {
	const (
		hwndBroadcast   = 0xFFFF
		wmSettingChange = 0x001A
		smtoAbortIfHung = 0x0002
		timeoutMS       = 5000
	)

	name, err := windows.UTF16PtrFromString(envKey)
	if err != nil {
		return
	}

	sendMessageTimeout.Call(
		hwndBroadcast,
		wmSettingChange,
		0,
		uintptr(unsafe.Pointer(name)),
		smtoAbortIfHung,
		timeoutMS,
		0,
	)
}

var sendMessageTimeout = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
