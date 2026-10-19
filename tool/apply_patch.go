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
	addResource := func(raw string, directoryEntry bool) error {
		if raw == "" {
			return nil
		}
		path, rel, err := ResolvePath(inv.WorkDir, raw)
		if err != nil {
			return err
		}
		if directoryEntry {
			// Removing a link changes its parent directory, not its target.
			_, parent, err := ResolvePath(inv.WorkDir, filepath.Dir(path))
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(filepath.Join(filepath.FromSlash(parent), filepath.Base(path)))
		}
		if !seen[rel] {
			seen[rel] = true
			resources = append(resources, rel)
		}
		return nil
	}
	for _, section := range sections {
		if err := addResource(section.Path, false); err != nil {
			return PermissionCheck{}, err
		}
		if section.Type == "delete" || section.MovePath != "" {
			if err := addResource(section.Path, true); err != nil {
				return PermissionCheck{}, err
			}
		}
		if err := addResource(section.MovePath, false); err != nil {
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

	var paths []string
	for _, section := range sections {
		for _, raw := range []string{section.Path, section.MovePath} {
			if raw == "" {
				continue
			}
			path, _, err := ResolvePath(inv.WorkDir, raw)
			if err != nil {
				return Result{}, err
			}
			paths = append(paths, path)
		}
	}
	unlock, err := lockMutation(ctx, paths...)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	seen := make(map[string]string)
	aliases := make(map[string]string)
	changes := make([]patchChange, 0, len(sections))
	for _, section := range sections {
		path, _, err := ResolvePath(inv.WorkDir, section.Path)
		if err != nil {
			return Result{}, err
		}
		recreate := section.Type == "add" && seen[path] == "delete"
		for _, raw := range []string{section.Path, section.MovePath} {
			if raw == "" {
				continue
			}
			target, _, err := ResolvePath(inv.WorkDir, raw)
			if err != nil {
				return Result{}, err
			}
			canonical, err := canonicalPath(target)
			if err != nil {
				return Result{}, err
			}
			if previous := aliases[canonical]; previous != "" && previous != target {
				return Result{}, fmt.Errorf("apply_patch verification failed: %s and %s address the same file", previous, target)
			}
			aliases[canonical] = target
			if seen[target] != "" && !(recreate && target == path) {
				return Result{}, fmt.Errorf("apply_patch verification failed: repeated target %s; combine its changes into one section", target)
			}
			seen[target] = section.Type
		}
		change, err := preparePatchSection(inv.WorkDir, section, recreate)
		if err != nil {
			return Result{}, err
		}
		changes = append(changes, change)
	}
	writes := make(map[string]bool)
	for _, section := range sections {
		if section.Type == "delete" {
			continue
		}
		raw := section.Path
		if section.MovePath != "" {
			raw = section.MovePath
		}
		path, _, err := ResolvePath(inv.WorkDir, raw)
		if err != nil {
			return Result{}, err
		}
		path, err = canonicalPath(path)
		if err != nil {
			return Result{}, err
		}
		writes[path] = true
	}
	for path := range writes {
		for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
			if writes[dir] {
				return Result{}, fmt.Errorf("apply_patch verification failed: file target %s is also a parent directory", dir)
			}
			info, err := os.Stat(dir)
			if err == nil && !info.IsDir() {
				return Result{}, fmt.Errorf("apply_patch verification failed: parent %s is not a directory", dir)
			}
			if err != nil && !os.IsNotExist(err) {
				return Result{}, err
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	var files []map[string]any
	var summaries []string
	for _, change := range changes {
		select {
		case <-ctx.Done():
			return Result{Text: "Applied before cancellation:\n" + strings.Join(summaries, "\n"), Metadata: map[string]any{"files": files}}, ctx.Err()
		default:
		}

		if err := change.commit(); err != nil {
			return Result{Text: "Applied before failure:\n" + strings.Join(summaries, "\n"), Metadata: map[string]any{"files": files}}, fmt.Errorf("apply_patch failed at %s: %w", change.RelativePath, err)
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
	commit       func() error
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

func preparePatchSection(workDir string, section patchSection, recreate bool) (patchChange, error) {
	path, rel, err := ResolvePath(workDir, section.Path)
	if err != nil {
		return patchChange{}, err
	}
	changeType := section.Type
	movePath := ""
	moveRel := ""
	if section.MovePath != "" {
		changeType = "move"
		movePath, moveRel, err = ResolvePath(workDir, section.MovePath)
		if err != nil {
			return patchChange{}, err
		}
		if _, err := os.Lstat(movePath); err == nil {
			return patchChange{}, fmt.Errorf("apply_patch verification failed: move target already exists: %s", section.MovePath)
		} else if !os.IsNotExist(err) {
			return patchChange{}, err
		}
	}

	var oldContent, newContent string
	switch section.Type {
	case "add":
		if _, err := os.Lstat(path); err == nil && !recreate {
			return patchChange{}, fmt.Errorf("apply_patch verification failed: file already exists: %s", section.Path)
		} else if err != nil && !os.IsNotExist(err) {
			return patchChange{}, err
		}
		for _, line := range section.Lines {
			if !strings.HasPrefix(line, "+") {
				return patchChange{}, fmt.Errorf("apply_patch verification failed: add file lines must start with +")
			}
			newContent += strings.TrimPrefix(line, "+") + "\n"
		}
	case "delete":
		for _, line := range section.Lines {
			if strings.TrimSpace(line) != "" {
				return patchChange{}, fmt.Errorf("apply_patch verification failed: delete sections cannot contain patch lines")
			}
		}
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
	mode := os.FileMode(0644)
	if section.Type != "add" {
		info, err := os.Stat(path)
		if err != nil {
			return patchChange{}, err
		}
		mode = info.Mode().Perm()
	}
	commit := func() error {
		switch section.Type {
		case "add", "update":
			target := path
			if movePath != "" {
				target = movePath
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(target, []byte(newContent), mode); err != nil {
				return err
			}
			if movePath != "" {
				if err := os.Remove(path); err != nil {
					return fmt.Errorf("wrote destination but could not remove source: %w", err)
				}
			}
		case "delete":
			if err := os.Remove(path); err != nil {
				return err
			}
		}

		return nil
	}
	return patchChange{commit: commit, Path: path, RelativePath: rel, MovePath: moveRel, Type: changeType, Patch: patch, Additions: additions, Deletions: deletions}, nil
}

func applyUpdateLines(oldContent string, patchLines []string) (string, error) {
	oldLines := splitLinesKeepEnd(strings.TrimPrefix(oldContent, utf8BOM))
	ending := textLineEnding(oldContent)
	var out []string
	lastAdded := false
	pos := 0
	for i := 0; i < len(patchLines); {
		if strings.HasPrefix(patchLines[i], "@@") {
			anchor := strings.TrimSpace(strings.TrimPrefix(patchLines[i], "@@"))
			if anchor != "" {
				idx := findLine(oldLines, pos, anchor)
				if idx < 0 {
					return "", fmt.Errorf("apply_patch verification failed: context not found: %s", anchor)
				}
				out = append(out, oldLines[pos:idx+1]...)
				lastAdded = false
				pos = idx + 1
			}
			i++
		}
		start := i
		var expected []string
		atEnd := false
		for i < len(patchLines) && !strings.HasPrefix(patchLines[i], "@@") {
			line := patchLines[i]
			if line == "*** End of File" {
				atEnd = true
				i++
				break
			}
			if len(line) == 0 || !strings.ContainsRune(" +-", rune(line[0])) {
				return "", fmt.Errorf("apply_patch verification failed: invalid hunk line %q", line)
			}
			if line[0] != '+' {
				expected = append(expected, line[1:])
			}
			i++
		}
		if i == start {
			return "", fmt.Errorf("apply_patch verification failed: empty hunk")
		}
		match := -1
		for index := pos; index+len(expected) <= len(oldLines); index++ {
			if atEnd && index+len(expected) != len(oldLines) {
				continue
			}
			found := true
			for j, text := range expected {
				if strings.TrimSuffix(strings.TrimSuffix(oldLines[index+j], "\r\n"), "\n") != text {
					found = false
					break
				}
			}
			if found {
				match = index
				break
			}
		}
		if match < 0 {
			return "", fmt.Errorf("apply_patch verification failed: contiguous context not found: %s", strings.Join(expected, "\n"))
		}
		out = append(out, oldLines[pos:match]...)
		if match > pos {
			lastAdded = false
		}
		pos = match
		for _, line := range patchLines[start:i] {
			if line == "*** End of File" {
				continue
			}
			switch line[0] {
			case ' ':
				out = append(out, oldLines[pos])
				lastAdded = false
				pos++
			case '-':
				pos++
			case '+':
				if len(out) > 0 && !strings.HasSuffix(out[len(out)-1], "\n") {
					out[len(out)-1] += ending
				}
				out = append(out, line[1:]+ending)
				lastAdded = true
			}
		}
	}
	out = append(out, oldLines[pos:]...)
	next := strings.Join(out, "")
	if lastAdded && pos == len(oldLines) && oldContent != "" && !strings.HasSuffix(oldContent, "\n") {
		next = strings.TrimSuffix(next, ending)
	}
	if strings.HasPrefix(oldContent, utf8BOM) {
		next = utf8BOM + next
	}
	return next, nil
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
		if strings.TrimSuffix(strings.TrimSuffix(lines[i], "\r\n"), "\n") == expected {
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
