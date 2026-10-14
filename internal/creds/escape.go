package creds

import (
	"os"
	"path/filepath"
	"strings"
)

// Escape renders a value as a double-quoted creds.conf field. Exact inverse of
// unescape: change one, change the other.
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

// Format builds the whole file body.
func Format(c Creds) string {
	return "USERNAME=" + Escape(c.Username) + "\nPASSWORD=" + Escape(c.Password) + "\n"
}

// DefaultPath is creds.conf beside the binary: triggers have no useful working directory.
func DefaultPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "creds.conf"
	}

	return filepath.Join(filepath.Dir(exe), "creds.conf")
}
