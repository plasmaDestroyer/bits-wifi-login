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

// HKCU\Environment is the per-user PATH: no elevation needed. Not setx, which
// silently truncates at 1024 chars, the classic way to wreck someone's PATH.
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

	// Keep REG_EXPAND_SZ, or %USERPROFILE% and friends stop expanding.
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

// pathNote says to close the terminal, not just open another: the shell that
// ran install keeps its old PATH no matter what the registry says.
func pathNote() string {
	return "\n  Note: close this terminal first, it still has the old PATH.\n"
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

// Case-insensitive, and entries may carry a trailing separator or quotes.
func samePath(a, b string) bool {
	clean := func(s string) string {
		s = strings.TrimSpace(s)
		s = strings.Trim(s, `"`)

		return strings.ToLower(strings.TrimRight(s, `\/`))
	}

	return clean(a) != "" && clean(a) == clean(b)
}

// announce tells Explorer the environment changed, or even new terminals get
// the old PATH until next sign-in. Best effort.
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
