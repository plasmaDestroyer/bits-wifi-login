//go:build windows

package installer

import "testing"

// Getting this wrong in the false direction appends a duplicate entry to the
// user's PATH on every install; getting it wrong in the true direction means
// the entry is never added at all.
func TestSamePath(t *testing.T) {
	const dir = `C:\Users\ASUS\AppData\Local\bits-wifi-login`

	for _, tc := range []struct {
		entry string
		same  bool
	}{
		{dir, true},
		{`c:\users\asus\appdata\local\bits-wifi-login`, true}, // Windows paths are case-insensitive
		{dir + `\`, true},       // a trailing separator is common in PATH
		{`"` + dir + `"`, true}, // and so are quotes
		{"  " + dir + "  ", true},
		{dir + `-old`, false},
		{`C:\Windows\System32`, false},
		{"", false},
	} {
		if got := samePath(tc.entry, dir); got != tc.same {
			t.Errorf("samePath(%q, dir) = %v, want %v", tc.entry, got, tc.same)
		}
	}
}

// An empty PATH entry must never match, or unlink would strip every empty
// segment and listed would think we are already installed.
func TestSamePathIgnoresEmptyEntries(t *testing.T) {
	if samePath("", "") {
		t.Error(`samePath("", "") = true — an empty PATH segment matched an empty directory`)
	}
}

func TestListed(t *testing.T) {
	const dir = `C:\Apps\bits-wifi-login`

	for _, tc := range []struct {
		path string
		has  bool
	}{
		{"", false},
		{`C:\Windows;C:\Windows\System32`, false},
		{dir, true},
		{`C:\Windows;` + dir, true},
		{dir + `;C:\Windows`, true},
		{`C:\Windows;` + dir + `\;C:\Windows\System32`, true},
		{`C:\Apps\bits-wifi-login-backup`, false},
	} {
		if got := listed(tc.path, dir); got != tc.has {
			t.Errorf("listed(%q, dir) = %v, want %v", tc.path, got, tc.has)
		}
	}
}
