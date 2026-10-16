package tool

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
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
	return "Read a file, directory, or saved tool-output reference. File contents are returned with line numbers; directories are returned as sorted entries."
}

func (t *ReadTool) Definition() Definition {
	return Definition{
		ReplaySafe:  true,
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"filePath": {
					Type:        "string",
					Description: "A file or directory path, or a tool-output: reference returned by a tool",
				},
				"offset": {
					Type:        "number",
					Description: "The line or directory entry number to start reading from (1-indexed)",
				},
				"limit": {
					Type:        "number",
					Description: "The maximum number of lines or directory entries to read (defaults to and capped at 2000)",
				},
			},
			Required: []string{"filePath"},
		},
	}
}

func (t *ReadTool) DirectoryScoped() {}

func (t *ReadTool) Execute(ctx context.Context, inv Invocation) (Result, error) {
	rawPath, _ := inv.Input["filePath"].(string)
	if rawPath == "" {
		return Result{}, fmt.Errorf("filePath is required")
	}

	if inv.WorkDir == "" {
		return Result{}, fmt.Errorf("workDir is required for read tool")
	}

	var path string
	var err error
	if strings.HasPrefix(rawPath, outputPrefix) {
		path, err = outputPath(inv, rawPath)
	} else {
		path, _, err = resolveWorkPath(inv.WorkDir, rawPath)
	}
	if err != nil {
		return Result{}, err
	}
	offset := intParam(inv.Input["offset"], 1)
	limit := intParam(inv.Input["limit"], defaultReadLimit)
	if offset < 1 {
		return Result{}, fmt.Errorf("offset must be >= 1")
	}
	if limit < 1 {
		return Result{}, fmt.Errorf("limit must be >= 1")
	}
	limit = min(limit, defaultReadLimit)

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
		return readDirectory(ctx, path, offset, limit)
	}
	return readFile(ctx, path, offset, limit)
}

func readDirectory(ctx context.Context, path string, offset, limit int) (Result, error) {
	sliced, total, err := directoryPage(ctx, path, offset, limit)
	if err != nil {
		return Result{}, fmt.Errorf("failed to read directory: %w", err)
	}
	start := offset - 1
	if start > total {
		return Result{}, fmt.Errorf("offset %d is out of range for this directory (%d entries)", offset, total)
	}
	bytes := 0
	for i, name := range sliced {
		if bytes+len(name)+1 > MaxOutputBytes-1024 {
			sliced = sliced[:i]
			break
		}
		bytes += len(name) + 1
	}
	end := start + len(sliced)
	truncated := end < total
	output := fmt.Sprintf("<path>%s</path>\n<type>directory</type>\n<entries>\n%s", path, strings.Join(sliced, "\n"))
	if truncated {
		output += fmt.Sprintf("\n(Showing %d of %d entries. Use offset=%d to continue.)", len(sliced), total, end+1)
	} else {
		output += fmt.Sprintf("\n(%d entries)", total)
	}
	output += "\n</entries>"
	return Result{Text: output, Metadata: map[string]any{"preview": strings.Join(firstStrings(sliced, maxReadPreviewLines), "\n"), "truncated": truncated}}, nil
}

func readFile(ctx context.Context, path string, offset, limit int) (Result, error) {
	file, err := os.Open(path)
	if err != nil {
		return Result{}, fmt.Errorf("failed to read file: %w", err)
	}
	defer file.Close()
	r := bufio.NewReader(file)
	start := offset - 1
	var rendered []string
	var preview []string
	total, bytes := 0, 0
	full := false
	for {
		lineLimit := maxReadLineLength
		if total+1 < offset || len(rendered) >= limit || full {
			lineLimit = 0
		}
		line, clipped, err := readLinePrefix(ctx, r, lineLimit)
		if err == io.EOF {
			break
		}
		if err != nil {
			return Result{}, err
		}
		total++
		if total < offset || len(rendered) >= limit || full {
			continue
		}
		if clipped {
			line += fmt.Sprintf("... (line truncated to %d bytes)", maxReadLineLength)
		}
		row := fmt.Sprintf("%d: %s", total, line)
		if bytes+len(row)+1 > MaxOutputBytes-1024 {
			full = true
			continue
		}
		bytes += len(row) + 1
		rendered = append(rendered, row)
		if len(preview) < maxReadPreviewLines {
			preview = append(preview, line)
		}
	}
	if start > total {
		return Result{}, fmt.Errorf("offset %d is out of range for this file (%d lines)", offset, total)
	}
	end := start + len(rendered)
	truncated := end < total
	output := fmt.Sprintf("<path>%s</path>\n<type>file</type>\n<content>\n%s", path, strings.Join(rendered, "\n"))
	if truncated {
		output += fmt.Sprintf("\n\n(Showing lines %d-%d of %d. Use offset=%d to continue.)", offset, end, total, end+1)
	} else {
		output += fmt.Sprintf("\n\n(End of file - total %d lines)", total)
	}
	output += "\n</content>"
	return Result{Text: output, Metadata: map[string]any{"preview": strings.Join(preview, "\n"), "truncated": truncated}}, nil
}

func readLinePrefix(ctx context.Context, r *bufio.Reader, limit int) (string, bool, error) {
	var line strings.Builder
	clipped, seen := false, false
	for n := 0; ; n++ {
		if n%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return "", false, err
			}
		}
		b, err := r.ReadByte()
		if err == io.EOF && seen {
			break
		}
		if err != nil {
			return "", false, err
		}
		seen = true
		if b == '\n' {
			break
		}
		if b == '\r' {
			if next, _ := r.Peek(1); len(next) > 0 && next[0] == '\n' {
				_, _ = r.ReadByte()
			}
			break
		}
		if line.Len() < limit {
			line.WriteByte(b)
		} else {
			clipped = true
		}
	}
	return validPrefix(line.String()), clipped, nil
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
