package tool

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type GrepTool struct{}

func NewGrepTool() *GrepTool     { return &GrepTool{} }
func (t *GrepTool) Name() string { return "grep" }
func (t *GrepTool) Description() string {
	return "Search text files with ripgrep ignore and glob rules. Skips binary files and returns matching lines with paths and line numbers. Requires rg."
}
func (t *GrepTool) Definition() Definition {
	return Definition{ReplaySafe: true, Name: t.Name(), Description: t.Description(), InputSchema: InputSchema{
		Type: "object", Properties: map[string]Property{
			"pattern":       {Type: "string", Description: "Ripgrep regular expression or literal text"},
			"path":          {Type: "string", Description: "File or directory to search; defaults to the working directory"},
			"include":       {Type: "string", Description: "File glob, for example *.{ts,tsx} or src/**/*.go"},
			"literal":       {Type: "boolean", Description: "Match literal text instead of a regular expression"},
			"caseSensitive": {Type: "boolean", Description: "Use case-sensitive matching (default: true)"},
			"hidden":        {Type: "boolean", Description: "Include hidden files and directories (default: false)"},
			"limit":         {Type: "integer", Description: "Maximum matching lines to return (default: 100)"},
		}, Required: []string{"pattern"},
	}}
}
func (t *GrepTool) DirectoryScoped() {}

func (t *GrepTool) Execute(ctx context.Context, inv Invocation) (Result, error) {
	pattern, _ := inv.Input["pattern"].(string)
	if pattern == "" {
		return Result{}, fmt.Errorf("pattern is required")
	}
	path, limit, err := searchOptions(inv)
	if err != nil {
		return Result{}, err
	}
	root, _, err := ResolvePath(inv.WorkDir, ".")
	if err != nil {
		return Result{}, err
	}
	args := []string{"--json"}
	if literal, _ := inv.Input["literal"].(bool); literal {
		args = append(args, "--fixed-strings")
	}
	if sensitive, ok := inv.Input["caseSensitive"].(bool); ok && !sensitive {
		args = append(args, "--ignore-case")
	} else {
		args = append(args, "--case-sensitive")
	}
	if include, _ := inv.Input["include"].(string); include != "" {
		args = append(args, "--glob="+include)
	}
	args = append(args, searchVisibility(inv)...)
	args = append(args, "--", pattern, path)
	output := &outputCollector{inv: inv}
	matches := 0
	err = runRipgrep(ctx, root, args, bufio.ScanLines, func(line []byte) error {
		var event struct {
			Type string `json:"type"`
			Data struct {
				Path  rgText `json:"path"`
				Lines rgText `json:"lines"`
				Line  int    `json:"line_number"`
			} `json:"data"`
		}
		if err := json.Unmarshal(line, &event); err != nil {
			return err
		}
		if event.Type != "match" {
			return nil
		}
		if matches == limit {
			return errSearchLimit
		}
		name := event.Data.Path.value()
		if rel, err := filepath.Rel(root, name); err == nil {
			name = rel
		}
		matches++
		_, err := fmt.Fprintf(output, "%s:%d: %s\n", filepath.ToSlash(name), event.Data.Line, strings.TrimRight(event.Data.Lines.value(), "\r\n"))
		return err
	})
	return finishSearch(output, err, "matches", matches, "No matches found for pattern: "+pattern)
}

type rgText struct {
	Text  string `json:"text"`
	Bytes string `json:"bytes"`
}

func (v rgText) value() string {
	if v.Bytes != "" {
		value, _ := base64.StdEncoding.DecodeString(v.Bytes)
		return string(value)
	}
	return v.Text
}
