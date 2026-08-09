package creds

import (
	"fmt"
	"os"
)

// Load reads creds.conf from disk. It tightens the file mode first, matching the
// defensive chmod the shell installer did — the file holds a plaintext password.
//
// On Windows the chmod is a no-op: Go maps os.Chmod onto the read-only attribute
// and there are no Unix mode bits to set. The file is protected there only by the
// default ACL on %LOCALAPPDATA%, which already excludes other standard users.
// Tightening it properly would mean going through the Windows ACL APIs; that is
// deliberately not done, but don't mistake this call for protection on Windows.
func Load(path string) (Creds, error) {
	_ = os.Chmod(path, 0600) // best-effort; a read-only file is still readable

	content, err := os.ReadFile(path)
	if err != nil {
		return Creds{}, fmt.Errorf("creds: %w", err)
	}

	return Parse(string(content))
}
