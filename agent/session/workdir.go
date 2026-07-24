package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveWorkDir validates a user-provided working directory and returns
// the resolved absolute path. An empty dir is returned as-is (no workdir).
// Non-empty dirs must exist and be directories.
func ResolveWorkDir(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", nil
	}
	if dir == "~" || strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if dir == "~" {
			dir = home
		} else {
			dir = filepath.Join(home, dir[2:])
		}
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
