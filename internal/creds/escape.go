package creds

import (
	"os"
	"path/filepath"
	"strings"
)

// Escape renders a value as a double-quoted creds.conf field. It is the exact
// inverse of unescape — the two must always be changed together, which is why
// they live side by side and are pinned by a round-trip test.
func Escape(value string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\r", `\r`,
		"\n", `\n`,
		"\t", `\t`,
	)

	return `"` + r.Replace(value) + `"`
}

// Format builds the whole file body, ready to write with mode 0600.
func Format(c Creds) string {
	return "USERNAME=" + Escape(c.Username) + "\nPASSWORD=" + Escape(c.Password) + "\n"
}

// DefaultPath resolves creds.conf next to the running binary. The triggers that
// run it have no useful working directory to resolve against.
func DefaultPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "creds.conf"
	}

	return filepath.Join(filepath.Dir(exe), "creds.conf")
}
