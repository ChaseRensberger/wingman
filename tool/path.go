package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chaserensberger/wingman/permission"
)

// ResolvePath returns an absolute path and its working-directory-relative or external absolute permission resource.
func ResolvePath(workDir, raw string) (string, string, error) {
	root, err := filepath.Abs(workDir)
	if err != nil {
		return "", "", err
	}
	root, err = canonicalPath(root)
	if err != nil {
		return "", "", err
	}
	path := filepath.FromSlash(permission.ExpandHome(raw))
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path = filepath.Clean(path)
	target, err := canonicalPath(path)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(root, target)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
		return path, filepath.ToSlash(rel), nil
	}
	return path, filepath.ToSlash(target), nil
}

func canonicalPath(path string) (string, error) {
	var suffix []string
	links := 0
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		// A dangling link still identifies the target of a future write.
		if target, linkErr := os.Readlink(path); linkErr == nil {
			links++
			if links > 255 {
				return "", fmt.Errorf("too many symbolic links: %s", path)
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(path), target)
			}
			path = filepath.Clean(target)
			continue
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		suffix = append(suffix, filepath.Base(path))
		path = parent
	}
}
