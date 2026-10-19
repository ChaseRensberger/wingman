package tool

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

type GlobTool struct{}

func NewGlobTool() *GlobTool     { return &GlobTool{} }
func (t *GlobTool) Name() string { return "glob" }
func (t *GlobTool) Description() string {
	return "Find files with ripgrep ignore and glob rules. Returns paths relative to the search directory."
}
func (t *GlobTool) Definition() Definition {
	return Definition{ReplaySafe: true, Name: t.Name(), Description: t.Description(), InputSchema: InputSchema{
		Type: "object", Properties: map[string]Property{
			"pattern": {Type: "string", Description: "Relative glob without .. components, such as src/**/*.{ts,tsx}"},
			"path":    {Type: "string", Description: "Directory to search; defaults to the working directory"},
			"hidden":  {Type: "boolean", Description: "Include hidden files and directories (default: false)"},
			"limit":   {Type: "integer", Description: "Maximum files to return (default: 100)"},
		}, Required: []string{"pattern"},
	}}
}
func (t *GlobTool) DirectoryScoped() {}
func (t *GlobTool) Execute(ctx context.Context, inv Invocation) (Result, error) {
	pattern, _ := inv.Input["pattern"].(string)
	if pattern == "" {
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
	path, limit, err := searchOptions(inv)
	if err != nil {
		return Result{}, err
	}
	args := append([]string{"--files", "--null", "--glob=" + pattern}, searchVisibility(inv)...)
	args = append(args, ".")
	output := &outputCollector{inv: inv}
	count := 0
	err = runRipgrep(ctx, path, args, func(data []byte, atEOF bool) (int, []byte, error) {
		if i := bytes.IndexByte(data, 0); i >= 0 {
			return i + 1, data[:i], nil
		}
		if atEOF && len(data) > 0 {
			return len(data), data, nil
		}
		return 0, nil, nil
	}, func(path []byte) error {
		if count == limit {
			return errSearchLimit
		}
		if count > 0 {
			if _, err := output.WriteString("\n"); err != nil {
				return err
			}
		}
		count++
		_, err := output.WriteString(strings.TrimPrefix(filepath.ToSlash(string(path)), "./"))
		return err
	})
	return finishSearch(output, err, "count", count, "No files found matching pattern: "+pattern)
}
