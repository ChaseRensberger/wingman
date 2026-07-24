package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

type ApplyPatchTool struct{}

func NewApplyPatchTool() *ApplyPatchTool { return &ApplyPatchTool{} }

func (t *ApplyPatchTool) Name() string { return "apply_patch" }

func (t *ApplyPatchTool) Description() string {
	return `Use the apply_patch tool to edit files. The patch language is a stripped-down, file-oriented diff format:

*** Begin Patch
[ one or more file sections ]
*** End Patch

Each operation starts with one of three headers:
*** Add File: <path> - create a new file. Every following line is a + line.
*** Delete File: <path> - remove an existing file.
*** Update File: <path> - patch an existing file in place, optionally followed by *** Move to: <path>.

Example:
*** Begin Patch
*** Add File: hello.txt
+Hello world
*** Update File: src/app.py
*** Move to: src/main.py
@@ def greet():
-print("Hi")
+print("Hello, world!")
*** Delete File: obsolete.txt
*** End Patch`
}

func (t *ApplyPatchTool) Definition() Definition {
	return Definition{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"patchText": {
					Type:        "string",
					Description: "The full patch text that describes all changes to be made",
				},
			},
			Required: []string{"patchText"},
		},
	}
}

func (t *ApplyPatchTool) DirectoryScoped() {}

func (t *ApplyPatchTool) Permission(inv Invocation) (PermissionCheck, error) {
	patchText, ok := inv.Input["patchText"].(string)
	if !ok || strings.TrimSpace(patchText) == "" {
		return PermissionCheck{}, fmt.Errorf("patchText is required")
	}
	if inv.WorkDir == "" {
		return PermissionCheck{}, fmt.Errorf("workDir is required for apply_patch tool")
	}
	sections, err := parsePatchSections(patchText)
	if err != nil {
		return PermissionCheck{}, err
	}
	seen := make(map[string]bool, len(sections)*2)
	resources := make([]string, 0, len(sections))
	addResource := func(raw string) error {
		if raw == "" {
			return nil
		}
		_, rel, err := resolveWorkPath(inv.WorkDir, raw)
		if err != nil {
			return err
		}
		if !seen[rel] {
			seen[rel] = true
			resources = append(resources, rel)
		}
		return nil
	}
	for _, section := range sections {
		if err := addResource(section.Path); err != nil {
			return PermissionCheck{}, err
		}
		if err := addResource(section.MovePath); err != nil {
			return PermissionCheck{}, err
		}
	}
	if len(resources) == 0 {
		resources = []string{"*"}
	}
	return PermissionCheck{Action: "edit", Resources: resources, Save: []string{"*"}}, nil
}

func (t *ApplyPatchTool) Execute(ctx context.Context, inv Invocation) (Result, error) {
	patchText, ok := inv.Input["patchText"].(string)
	if !ok || strings.TrimSpace(patchText) == "" {
		return Result{}, fmt.Errorf("patchText is required")
	}
	if inv.WorkDir == "" {
		return Result{}, fmt.Errorf("workDir is required for apply_patch tool")
	}

	sections, err := parsePatchSections(patchText)
	if err != nil {
		return Result{}, err
	}

	var files []map[string]any
	var summaries []string
	for _, section := range sections {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		default:
		}

		change, err := applyPatchSection(inv.WorkDir, section)
		if err != nil {
			return Result{}, err
		}
		files = append(files, map[string]any{
			"filePath":     change.Path,
			"relativePath": change.RelativePath,
			"type":         change.Type,
			"patch":        change.Patch,
			"additions":    change.Additions,
			"deletions":    change.Deletions,
			"movePath":     change.MovePath,
		})
		summaryPath := change.RelativePath
		if change.MovePath != "" {
			summaryPath += " -> " + change.MovePath
		}
		summaries = append(summaries, fmt.Sprintf("%s %s", patchSummaryPrefix(change.Type), summaryPath))
	}

	output := "Success. Updated the following files:\n" + strings.Join(summaries, "\n")
	return Result{Text: output, Metadata: map[string]any{"files": files}}, nil
}

func fileDiffMetadata(path, rel, kind, patch string, additions, deletions int) map[string]any {
	return map[string]any{
		"files": []map[string]any{{
			"filePath":     path,
			"relativePath": rel,
			"type":         kind,
			"patch":        patch,
			"additions":    additions,
			"deletions":    deletions,
		}},
	}
}

type patchSection struct {
	Type     string
	Path     string
	MovePath string
	Lines    []string
}

type patchChange struct {
	Path         string
	RelativePath string
	MovePath     string
	Type         string
	Patch        string
	Additions    int
	Deletions    int
}

func parsePatchSections(text string) ([]patchSection, error) {
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "*** Begin Patch" {
		return nil, fmt.Errorf("apply_patch verification failed: patch must start with *** Begin Patch")
	}
	if strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) < 2 || strings.TrimSpace(lines[len(lines)-1]) != "*** End Patch" {
		return nil, fmt.Errorf("apply_patch verification failed: patch must end with *** End Patch")
	}

	var sections []patchSection
	var current *patchSection
	for _, line := range lines[1 : len(lines)-1] {
		switch {
		case strings.HasPrefix(line, "*** Add File: "):
			if current != nil {
				sections = append(sections, *current)
			}
			current = &patchSection{Type: "add", Path: strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: "))}
		case strings.HasPrefix(line, "*** Delete File: "):
			if current != nil {
				sections = append(sections, *current)
			}
			current = &patchSection{Type: "delete", Path: strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))}
		case strings.HasPrefix(line, "*** Update File: "):
			if current != nil {
				sections = append(sections, *current)
			}
			current = &patchSection{Type: "update", Path: strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: "))}
		case strings.HasPrefix(line, "*** Move to: "):
			if current == nil || current.Type != "update" {
				return nil, fmt.Errorf("apply_patch verification failed: Move to must follow Update File")
			}
			current.MovePath = strings.TrimSpace(strings.TrimPrefix(line, "*** Move to: "))
		default:
			if current == nil {
				if strings.TrimSpace(line) == "" {
					continue
				}
				return nil, fmt.Errorf("apply_patch verification failed: content before file header")
			}
			current.Lines = append(current.Lines, line)
		}
	}
	if current != nil {
		sections = append(sections, *current)
	}
	if len(sections) == 0 {
		return nil, fmt.Errorf("apply_patch verification failed: no file sections found")
	}
	for _, section := range sections {
		if section.Path == "" {
			return nil, fmt.Errorf("apply_patch verification failed: file path is required")
		}
	}
	return sections, nil
}

func applyPatchSection(workDir string, section patchSection) (patchChange, error) {
	path, rel, err := resolveWorkPath(workDir, section.Path)
	if err != nil {
		return patchChange{}, err
	}
	changeType := section.Type
	movePath := ""
	moveRel := ""
	if section.MovePath != "" {
		changeType = "move"
		movePath, moveRel, err = resolveWorkPath(workDir, section.MovePath)
		if err != nil {
			return patchChange{}, err
		}
	}

	var oldContent, newContent string
	switch section.Type {
	case "add":
		if _, err := os.Stat(path); err == nil {
			return patchChange{}, fmt.Errorf("apply_patch verification failed: file already exists: %s", section.Path)
		}
		for _, line := range section.Lines {
			if !strings.HasPrefix(line, "+") {
				return patchChange{}, fmt.Errorf("apply_patch verification failed: add file lines must start with +")
			}
			newContent += strings.TrimPrefix(line, "+") + "\n"
		}
	case "delete":
		bytes, err := os.ReadFile(path)
		if err != nil {
			return patchChange{}, fmt.Errorf("apply_patch verification failed: failed to read file to delete: %w", err)
		}
		oldContent = string(bytes)
	case "update":
		bytes, err := os.ReadFile(path)
		if err != nil {
			return patchChange{}, fmt.Errorf("apply_patch verification failed: failed to read file to update: %w", err)
		}
		oldContent = string(bytes)
		newContent, err = applyUpdateLines(oldContent, section.Lines)
		if err != nil {
			return patchChange{}, err
		}
	default:
		return patchChange{}, fmt.Errorf("apply_patch verification failed: unknown section type %q", section.Type)
	}

	patch, additions, deletions := unifiedPatch(rel, oldContent, newContent)
	switch section.Type {
	case "add", "update":
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return patchChange{}, fmt.Errorf("apply_patch failed: create parent directories: %w", err)
		}
		if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
			return patchChange{}, fmt.Errorf("apply_patch failed: write file: %w", err)
		}
		if movePath != "" {
			if err := os.MkdirAll(filepath.Dir(movePath), 0755); err != nil {
				return patchChange{}, fmt.Errorf("apply_patch failed: create move parent directories: %w", err)
			}
			if err := os.WriteFile(movePath, []byte(newContent), 0644); err != nil {
				return patchChange{}, fmt.Errorf("apply_patch failed: write moved file: %w", err)
			}
			if err := os.Remove(path); err != nil {
				return patchChange{}, fmt.Errorf("apply_patch failed: remove original file: %w", err)
			}
		}
	case "delete":
		if err := os.Remove(path); err != nil {
			return patchChange{}, fmt.Errorf("apply_patch failed: delete file: %w", err)
		}
	}

	return patchChange{Path: path, RelativePath: rel, MovePath: moveRel, Type: changeType, Patch: patch, Additions: additions, Deletions: deletions}, nil
}

func applyUpdateLines(oldContent string, patchLines []string) (string, error) {
	oldLines := splitLinesKeepEnd(oldContent)
	var out []string
	pos := 0
	for i := 0; i < len(patchLines); {
		line := patchLines[i]
		if strings.HasPrefix(line, "@@") {
			anchor := strings.TrimSpace(strings.TrimPrefix(line, "@@"))
			if anchor != "" {
				idx := findLine(oldLines, pos, anchor)
				if idx < 0 {
					return "", fmt.Errorf("apply_patch verification failed: context not found: %s", anchor)
				}
				out = append(out, oldLines[pos:idx+1]...)
				pos = idx + 1
			}
			i++
			continue
		}

		if strings.HasPrefix(line, "+") {
			out = append(out, strings.TrimPrefix(line, "+")+"\n")
			i++
			continue
		}

		if !strings.HasPrefix(line, "-") && !strings.HasPrefix(line, " ") {
			return "", fmt.Errorf("apply_patch verification failed: update lines must start with +, -, space, or @@")
		}

		expected := line[1:]
		idx := findLine(oldLines, pos, expected)
		if idx < 0 {
			return "", fmt.Errorf("apply_patch verification failed: context not found: %s", expected)
		}
		out = append(out, oldLines[pos:idx]...)
		if strings.HasPrefix(line, " ") {
			out = append(out, oldLines[idx])
		}
		pos = idx + 1
		i++
	}
	out = append(out, oldLines[pos:]...)
	return strings.Join(out, ""), nil
}

func splitLinesKeepEnd(text string) []string {
	if text == "" {
		return nil
	}
	parts := strings.SplitAfter(text, "\n")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func findLine(lines []string, start int, expected string) int {
	for i := start; i < len(lines); i++ {
		if strings.TrimSuffix(lines[i], "\n") == expected {
			return i
		}
	}
	return -1
}

func unifiedPatch(name, oldContent, newContent string) (string, int, int) {
	diff := difflib.UnifiedDiff{
		A:        splitDiffLines(oldContent),
		B:        splitDiffLines(newContent),
		FromFile: "a/" + name,
		ToFile:   "b/" + name,
		Context:  3,
	}
	patch, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		// Fallback: return a minimal marker so consumers don't crash.
		return fmt.Sprintf("--- a/%s\n+++ b/%s\n", name, name), 0, 0
	}
	additions, deletions := 0, 0
	inHunk := false
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "@@") {
			inHunk = true
			continue
		}
		if !inHunk {
			continue
		}
		if strings.HasPrefix(line, "+") {
			additions++
		}
		if strings.HasPrefix(line, "-") {
			deletions++
		}
	}
	return patch, additions, deletions
}

func splitDiffLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.SplitAfter(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	} else if !strings.HasSuffix(lines[len(lines)-1], "\n") {
		lines[len(lines)-1] += "\n\\ No newline at end of file\n"
	}
	return lines
}

func patchSummaryPrefix(kind string) string {
	switch kind {
	case "add":
		return "A"
	case "delete":
		return "D"
	default:
		return "M"
	}
}

func resolveWorkPath(workDir, raw string) (string, string, error) {
	path := raw
	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, path)
	}
	path = filepath.Clean(path)
	workDir = filepath.Clean(workDir)
	rel, err := filepath.Rel(workDir, path)
	if err != nil {
		return "", "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path escapes working directory: %s", raw)
	}
	return path, filepath.ToSlash(rel), nil
}
