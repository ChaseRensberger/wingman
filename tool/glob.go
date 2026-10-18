package tool

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
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

func (t *GlobTool) Definition() Definition {
	return Definition{
		ReplaySafe:  true,
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"pattern": {
					Type:        "string",
					Description: "A relative glob pattern without .. components (e.g., '**/*.go', 'src/**/*.ts'). Use path to select another directory.",
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

func (t *GlobTool) DirectoryScoped() {}

func (t *GlobTool) Execute(ctx context.Context, inv Invocation) (Result, error) {
	pattern, ok := inv.Input["pattern"].(string)
	if !ok || pattern == "" {
		return Result{}, fmt.Errorf("pattern is required")
	}
	if filepath.IsAbs(pattern) {
		return Result{}, fmt.Errorf("pattern must be relative; use path to select a directory")
	}
	for _, part := range strings.Split(filepath.ToSlash(pattern), "/") {
		if part == ".." {
			return Result{}, fmt.Errorf("pattern cannot contain ..; use path to select a directory")
		}
	}

	if inv.WorkDir == "" {
		return Result{}, fmt.Errorf("workDir is required for glob tool")
	}

	path, _ := inv.Input["path"].(string)
	if path == "" {
		path = "."
	}
	baseDir, _, err := ResolvePath(inv.WorkDir, path)
	if err != nil {
		return Result{}, err
	}
	baseDir, err = canonicalPath(baseDir)
	if err != nil {
		return Result{}, err
	}

	output := &outputCollector{inv: inv}
	matches := 0
	emit := func(path string) error {
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}
		if matches > 0 {
			if _, err := output.Write([]byte("\n")); err != nil {
				return err
			}
		}
		matches++
		_, err = output.Write([]byte(rel))
		return err
	}
	if strings.Contains(pattern, "**") {
		err = walkFiles(ctx, baseDir, nil, func(path string) error {
			relPath, err := filepath.Rel(baseDir, path)
			if err != nil {
				return nil
			}

			if matchDoubleGlob(pattern, relPath) {
				return emit(path)
			}
			return nil
		})
	} else {
		fullPattern := filepath.Join(baseDir, pattern)
		err = globFiles(ctx, fullPattern, emit)
	}
	result, outputErr := output.finish(map[string]any{"count": matches})
	if err != nil {
		return result, err
	}
	if outputErr != nil {
		return result, outputErr
	}
	if matches == 0 {
		return Result{Text: "No files found matching pattern: " + pattern, Metadata: map[string]any{"count": 0}}, nil
	}

	return result, nil
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
