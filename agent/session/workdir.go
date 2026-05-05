package session

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveWorkDir validates a user-provided working directory and returns
// the resolved absolute path. An empty dir is returned as-is (no workdir).
// Non-empty dirs must exist and be directories.
func ResolveWorkDir(dir string) (string, error) {
	if dir == "" {
		return "", nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("invalid working directory %q: %w", dir, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("working directory %q does not exist", abs)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("working directory %q is not a directory", abs)
	}
	return abs, nil
}
