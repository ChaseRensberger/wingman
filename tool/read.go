package tool

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	defaultReadLimit    = 2000
	maxReadLineLength   = 2000
	maxReadPreviewLines = 20
)

type ReadTool struct{}

func NewReadTool() *ReadTool {
	return &ReadTool{}
}

func (t *ReadTool) Name() string {
	return "read"
}

func (t *ReadTool) Description() string {
	return "Read a file or directory from the local filesystem. File contents are returned with line numbers; directories are returned as sorted entries."
}

func (t *ReadTool) Definition() Definition {
	return Definition{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"filePath": {
					Type:        "string",
					Description: "The path to the file or directory to read (relative to working directory or absolute)",
				},
				"offset": {
					Type:        "number",
					Description: "The line or directory entry number to start reading from (1-indexed)",
				},
				"limit": {
					Type:        "number",
					Description: "The maximum number of lines or directory entries to read (defaults to 2000)",
				},
			},
			Required: []string{"filePath"},
		},
	}
}

func (t *ReadTool) DirectoryScoped() {}

func (t *ReadTool) Execute(ctx context.Context, params map[string]any, workDir string) (Result, error) {
	rawPath, _ := params["filePath"].(string)
	if rawPath == "" {
		return Result{}, fmt.Errorf("filePath is required")
	}

	if workDir == "" {
		return Result{}, fmt.Errorf("workDir is required for read tool")
	}

	path, _, err := resolveWorkPath(workDir, rawPath)
	if err != nil {
		return Result{}, err
	}
	offset := intParam(params["offset"], 1)
	limit := intParam(params["limit"], defaultReadLimit)
	if offset < 1 {
		return Result{}, fmt.Errorf("offset must be >= 1")
	}
	if limit < 1 {
		return Result{}, fmt.Errorf("limit must be >= 1")
	}

	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	default:
	}

	info, err := os.Stat(path)
	if err != nil {
		return Result{}, fmt.Errorf("failed to read path: %w", err)
	}
	if info.IsDir() {
		return readDirectory(path, offset, limit)
	}
	return readFile(path, offset, limit)
}

func readDirectory(path string, offset, limit int) (Result, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return Result{}, fmt.Errorf("failed to read directory: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	start := offset - 1
	if start > len(names) {
		return Result{}, fmt.Errorf("offset %d is out of range for this directory (%d entries)", offset, len(names))
	}
	end := start + limit
	if end > len(names) {
		end = len(names)
	}
	sliced := names[start:end]
	truncated := end < len(names)
	output := fmt.Sprintf("<path>%s</path>\n<type>directory</type>\n<entries>\n%s", path, strings.Join(sliced, "\n"))
	if truncated {
		output += fmt.Sprintf("\n(Showing %d of %d entries. Use offset=%d to continue.)", len(sliced), len(names), end+1)
	} else {
		output += fmt.Sprintf("\n(%d entries)", len(names))
	}
	output += "\n</entries>"
	return Result{Text: output, Metadata: map[string]any{"preview": strings.Join(firstStrings(sliced, maxReadPreviewLines), "\n"), "truncated": truncated}}, nil
}

func readFile(path string, offset, limit int) (Result, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("failed to read file: %w", err)
	}
	text := string(content)
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	start := offset - 1
	if start > len(lines) {
		return Result{}, fmt.Errorf("offset %d is out of range for this file (%d lines)", offset, len(lines))
	}
	end := start + limit
	if end > len(lines) {
		end = len(lines)
	}
	var rendered []string
	for i, line := range lines[start:end] {
		if len(line) > maxReadLineLength {
			line = line[:maxReadLineLength] + fmt.Sprintf("... (line truncated to %d chars)", maxReadLineLength)
		}
		rendered = append(rendered, fmt.Sprintf("%d: %s", start+i+1, line))
	}
	truncated := end < len(lines)
	output := fmt.Sprintf("<path>%s</path>\n<type>file</type>\n<content>\n%s", path, strings.Join(rendered, "\n"))
	if truncated {
		output += fmt.Sprintf("\n\n(Showing lines %d-%d of %d. Use offset=%d to continue.)", offset, end, len(lines), end+1)
	} else {
		output += fmt.Sprintf("\n\n(End of file - total %d lines)", len(lines))
	}
	output += "\n</content>"
	return Result{Text: output, Metadata: map[string]any{"preview": strings.Join(firstStrings(lines[start:end], maxReadPreviewLines), "\n"), "truncated": truncated}}, nil
}

func intParam(value any, fallback int) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	default:
		return fallback
	}
}

func firstStrings(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}
