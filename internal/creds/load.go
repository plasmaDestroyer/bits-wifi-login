package creds

import (
	"fmt"
	"os"
)

// Load reads creds.conf, tightening it to 0600 first (plaintext password).
// On Windows chmod only flips the read-only attribute. The %LOCALAPPDATA% ACL
// is the real guard.
func Load(path string) (Creds, error) {
	_ = os.Chmod(path, 0600)

	content, err := os.ReadFile(path)
	if err != nil {
		return Creds{}, fmt.Errorf("creds: %w", err)
	}

	return Parse(string(content))
}
