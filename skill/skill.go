// Package skill discovers and executes local Agent Skills.
package skill

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chaserensberger/wingman/tool"
	"go.yaml.in/yaml/v4"
)

// ToolName is reserved for Wingman's native skill loader.
const ToolName = "skill"

// Skill is an immutable local skill snapshot.
type Skill struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Description     string           `json:"description,omitempty"`
	Content         string           `json:"content"`
	Location        string           `json:"location"`
	BaseDir         string           `json:"base_dir"`
	SHA256          string           `json:"sha256"`
	SupportingFiles []SupportingFile `json:"supporting_files,omitempty"`
}

// SupportingFile is immutable supporting content captured with a skill.
type SupportingFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	SHA256  string `json:"sha256"`
}

// Discover loads root Markdown files and nested SKILL.md files. Later roots
// override earlier roots when they contain the same skill ID.
func Discover(dirs ...string) ([]Skill, error) {
	byID := map[string]Skill{}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		absolute, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(absolute); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(absolute)
		if err != nil {
			return nil, err
		}
		source := map[string]Skill{}
		add := func(s Skill) error {
			if _, exists := source[s.ID]; exists {
				return fmt.Errorf("duplicate skill ID %q in %q", s.ID, absolute)
			}
			source[s.ID] = s
			return nil
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
				s, err := load(filepath.Join(absolute, entry.Name()), strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())), absolute, false)
				if err != nil {
					return nil, err
				}
				if err := add(s); err != nil {
					return nil, err
				}
				continue
			}
			if !entry.IsDir() {
				continue
			}
			err := filepath.WalkDir(filepath.Join(absolute, entry.Name()), func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() || d.Name() != "SKILL.md" {
					return nil
				}
				s, err := load(path, filepath.Base(filepath.Dir(path)), filepath.Dir(path), true)
				if err != nil {
					return err
				}
				return add(s)
			})
			if err != nil {
				return nil, err
			}
		}
		for id, s := range source {
			byID[id] = s
		}
	}
	out := make([]Skill, 0, len(byID))
	for _, s := range byID {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func load(path, id, baseDir string, includeSupportingFiles bool) (Skill, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Skill{}, err
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return Skill{}, err
	}
	base, err := filepath.EvalSymlinks(baseDir)
	if err != nil {
		return Skill{}, err
	}
	data, err := os.ReadFile(canonical)
	if err != nil {
		return Skill{}, err
	}
	name, description, content, err := parse(data)
	if err != nil {
		return Skill{}, fmt.Errorf("parse skill %q: %w", canonical, err)
	}
	if name == "" {
		name = id
	}
	sum := sha256.Sum256(data)
	var files []SupportingFile
	if includeSupportingFiles {
		files, err = supportingFiles(base, canonical)
		if err != nil {
			return Skill{}, err
		}
	}
	return Skill{ID: id, Name: name, Description: description, Content: content, Location: canonical, BaseDir: base, SHA256: fmt.Sprintf("%x", sum), SupportingFiles: files}, nil
}

func parse(data []byte) (string, string, string, error) {
	content := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return "", "", content, nil
	}
	rest := content[4:]
	end := strings.Index(rest, "\n---\n")
	delimiterLength := len("\n---\n")
	if end < 0 && strings.HasSuffix(rest, "\n---") {
		end = len(rest) - len("\n---")
		delimiterLength = len("\n---")
	}
	if end < 0 {
		return "", "", "", fmt.Errorf("unterminated frontmatter")
	}
	var front struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &front); err != nil {
		return "", "", "", err
	}
	return front.Name, front.Description, rest[end+delimiterLength:], nil
}

func supportingFiles(base, skillFile string) ([]SupportingFile, error) {
	var paths []string
	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			if path != base {
				if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err == nil {
					return filepath.SkipDir
				} else if !errors.Is(err, fs.ErrNotExist) {
					return err
				}
			}
			return nil
		}
		if path == skillFile || d.Name() == "SKILL.md" {
			return nil
		}
		relative, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	if len(paths) > 10 {
		paths = paths[:10]
	}
	files := make([]SupportingFile, 0, len(paths))
	for _, relative := range paths {
		path := filepath.Join(base, filepath.FromSlash(relative))
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, err
		}
		contained, err := filepath.Rel(base, canonical)
		if err != nil || contained == ".." || strings.HasPrefix(contained, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("supporting file %q escapes the skill directory", relative)
		}
		data, err := os.ReadFile(canonical)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		files = append(files, SupportingFile{Path: relative, Content: string(data), SHA256: fmt.Sprintf("%x", sum)})
	}
	return files, nil
}

type nativeTool struct{ skills map[string]Skill }

// Tool returns the native skill tool for a snapshot.
func Tool(skills []Skill) tool.Tool {
	byID := make(map[string]Skill, len(skills))
	for _, s := range skills {
		byID[s.ID] = s
	}
	return &nativeTool{skills: byID}
}

func (t *nativeTool) Name() string { return ToolName }
func (t *nativeTool) Description() string {
	return "Load a local Agent Skill by ID, or read one of its supporting files."
}
func (t *nativeTool) Definition() tool.Definition {
	return tool.Definition{Name: t.Name(), Description: t.Description(), InputSchema: tool.InputSchema{Type: "object", Properties: map[string]tool.Property{
		"id":   {Type: "string", Description: "Skill ID"},
		"file": {Type: "string", Description: "Optional supporting file path relative to the skill's base directory"},
	}, Required: []string{"id"}}, Permission: &tool.PermissionTarget{Action: "skill", ResourceFields: []string{"id"}}}
}
func (t *nativeTool) Execute(_ context.Context, inv tool.Invocation) (tool.Result, error) {
	id, _ := inv.Input["id"].(string)
	s, ok := t.skills[id]
	if !ok {
		return tool.Result{}, fmt.Errorf("skill %q not found", id)
	}
	if file, _ := inv.Input["file"].(string); file != "" {
		return readSupportingFile(s, file)
	}
	text := fmt.Sprintf("# %s\n\nBase directory: %s\nSkill file: %s\n", s.Name, s.BaseDir, s.Location)
	if len(s.SupportingFiles) > 0 {
		paths := make([]string, len(s.SupportingFiles))
		for i, file := range s.SupportingFiles {
			paths[i] = file.Path
		}
		text += "Supporting files (load with the skill tool's file input):\n- " + strings.Join(paths, "\n- ") + "\n"
	}
	text += "\n" + s.Content
	return tool.Result{Text: text}, nil
}

func readSupportingFile(s Skill, file string) (tool.Result, error) {
	clean := filepath.Clean(filepath.FromSlash(file))
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return tool.Result{}, fmt.Errorf("supporting file must be relative to the skill directory")
	}
	wanted := filepath.ToSlash(clean)
	var found *SupportingFile
	for _, candidate := range s.SupportingFiles {
		if candidate.Path == wanted {
			candidate := candidate
			found = &candidate
			break
		}
	}
	if found == nil {
		return tool.Result{}, fmt.Errorf("supporting file %q not found for skill %q", file, s.ID)
	}
	return tool.Result{Text: fmt.Sprintf("<skill_file skill=%q path=%q>\n%s\n</skill_file>", s.ID, wanted, found.Content)}, nil
}
