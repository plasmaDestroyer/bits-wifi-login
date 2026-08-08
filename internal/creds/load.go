package creds

import (
	"fmt"
	"os"
)

// Load reads creds.conf from disk. It tightens the file mode first, matching the
// defensive chmod the shell installer did — the file holds a plaintext password.
func Load(path string) (Creds, error) {
	_ = os.Chmod(path, 0600) // best-effort; a read-only file is still readable

	content, err := os.ReadFile(path)
	if err != nil {
		return Creds{}, fmt.Errorf("creds: %w", err)
	}

	return Parse(string(content))
}
