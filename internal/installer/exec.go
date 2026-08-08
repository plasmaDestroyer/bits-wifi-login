package installer

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin // sudo may need to prompt

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installer: %s %s: %w", name, strings.Join(args, " "), err)
	}

	return nil
}

func write(path, content string, mode os.FileMode) error {
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		return fmt.Errorf("installer: writing %s: %w", path, err)
	}

	return nil
}
