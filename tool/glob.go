package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chaserensberger/wingman/models"
)

type GlobTool struct{}

func NewGlobTool() *GlobTool {
	return &GlobTool{}
}

func (t *GlobTool) Name() string {
	return "glob"
}

func (t *GlobTool) Description() string {
	return "Find files matching a glob pattern. Returns a list of matching file paths relative to the working directory."
}

func (t *GlobTool) Definition() models.WingmanToolDefinition {
	return models.WingmanToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: models.WingmanToolInputSchema{
			Type: "object",
			Properties: map[string]models.WingmanToolProperty{
				"pattern": {
					Type:        "string",
					Description: "The glob pattern to match (e.g., '**/*.go', 'src/**/*.ts', '*.json')",
				},
				"path": {
					Type:        "string",
					Description: "Base directory to search in (optional, defaults to working directory)",
				},
			},
			Required: []string{"pattern"},
		},
	}
}

func (t *GlobTool) Execute(ctx context.Context, params map[string]any, workDir string) (string, error) {
	pattern, ok := params["pattern"].(string)
	if !ok || pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}

	if workDir == "" {
		return "", fmt.Errorf("workDir is required for glob tool")
	}

	baseDir := workDir
	if path, ok := params["path"].(string); ok && path != "" {
		if filepath.IsAbs(path) {
			baseDir = path
		} else {
			baseDir = filepath.Join(workDir, path)
		}
	}

	var matches []string

	if strings.Contains(pattern, "**") {
		err := filepath.WalkDir(baseDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}

			relPath, err := filepath.Rel(baseDir, path)
			if err != nil {
				return nil
			}

			if matchDoubleGlob(pattern, relPath) {
				matches = append(matches, relPath)
			}
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("failed to walk directory: %w", err)
		}
	} else {
		fullPattern := filepath.Join(baseDir, pattern)
		results, err := filepath.Glob(fullPattern)
		if err != nil {
			return "", fmt.Errorf("invalid glob pattern: %w", err)
		}
		for _, path := range results {
			relPath, err := filepath.Rel(baseDir, path)
			if err == nil {
				matches = append(matches, relPath)
			}
		}
	}

	if len(matches) == 0 {
		return "No files found matching pattern: " + pattern, nil
	}

	return strings.Join(matches, "\n"), nil
}

func matchDoubleGlob(pattern, path string) bool {
	parts := strings.Split(pattern, "**")
	if len(parts) != 2 {
		matched, _ := filepath.Match(pattern, path)
		return matched
	}

	prefix := parts[0]
	suffix := parts[1]

	if prefix != "" {
		prefix = strings.TrimSuffix(prefix, "/")
		if !strings.HasPrefix(path, prefix) && prefix != "" {
			return false
		}
	}

	if suffix != "" {
		suffix = strings.TrimPrefix(suffix, "/")
		matched, _ := filepath.Match(suffix, filepath.Base(path))
		if !matched {
			dirMatched, _ := filepath.Match("*"+suffix, path)
			return dirMatched
		}
		return matched
	}

	return true
}
