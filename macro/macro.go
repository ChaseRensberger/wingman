// Package macro discovers and expands project macros.
package macro

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v4"
)

const directoryName = "macros"

var shellInterpolation = regexp.MustCompile("!`[^`]*`")
var placeholder = regexp.MustCompile(`\$(ARGUMENTS|[0-9]+)`)

// Macro is one immutable project macro definition.
type Macro struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
	ModelRef    string `json:"model_ref,omitempty"`
	Template    string `json:"-"`
}

// Discover loads Markdown macros from a working directory.
func Discover(workDir string) ([]Macro, error) {
	root := filepath.Join(workDir, ".wingman", directoryName)
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return []Macro{}, nil
		}
		return nil, err
	}

	macros := make([]Macro, 0)
	byID := map[string]struct{}{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		id := strings.TrimSuffix(filepath.ToSlash(relative), ".md")
		if _, exists := byID[id]; exists {
			return fmt.Errorf("duplicate macro ID %q", id)
		}
		definition, err := load(path, id)
		if err != nil {
			return err
		}
		byID[id] = struct{}{}
		macros = append(macros, definition)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(macros, func(i, j int) bool { return macros[i].ID < macros[j].ID })
	return macros, nil
}

// Expand applies macro arguments to its template.
func Expand(macro Macro, arguments string) string {
	arguments = strings.TrimSpace(arguments)
	args := parseArguments(arguments)
	highest := 0
	for _, match := range placeholder.FindAllStringSubmatch(macro.Template, -1) {
		if match[1] == "ARGUMENTS" {
			continue
		}
		var position int
		_, _ = fmt.Sscanf(match[1], "%d", &position)
		if position > highest {
			highest = position
		}
	}
	expanded := placeholder.ReplaceAllStringFunc(macro.Template, func(match string) string {
		name := strings.TrimPrefix(match, "$")
		if name == "ARGUMENTS" {
			return arguments
		}
		var position int
		_, _ = fmt.Sscanf(name, "%d", &position)
		index := position - 1
		if index < 0 || index >= len(args) {
			return ""
		}
		if position == highest {
			return strings.Join(args[index:], " ")
		}
		return args[index]
	})
	if highest == 0 && !strings.Contains(macro.Template, "$ARGUMENTS") && arguments != "" {
		expanded += "\n\n" + arguments
	}
	return strings.TrimSpace(expanded)
}

func load(path, id string) (Macro, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Macro{}, err
	}
	template, frontmatter, err := splitFrontmatter(string(data))
	if err != nil {
		return Macro{}, fmt.Errorf("parse macro %q: %w", path, err)
	}
	if strings.TrimSpace(template) == "" {
		return Macro{}, fmt.Errorf("macro %q has an empty template", path)
	}
	if shellInterpolation.MatchString(template) {
		return Macro{}, fmt.Errorf("macro %q contains shell interpolation", path)
	}
	var fields struct {
		Description string `yaml:"description"`
		AgentID     string `yaml:"agent"`
		ModelRef    string `yaml:"model"`
	}
	if frontmatter != "" {
		decoder := yaml.NewDecoder(strings.NewReader(frontmatter))
		decoder.KnownFields(true)
		if err := decoder.Decode(&fields); err != nil {
			return Macro{}, fmt.Errorf("parse macro %q frontmatter: %w", path, err)
		}
	}
	return Macro{ID: id, Description: fields.Description, AgentID: fields.AgentID, ModelRef: fields.ModelRef, Template: strings.TrimSpace(template)}, nil
}

func splitFrontmatter(content string) (template, frontmatter string, err error) {
	content = strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return content, "", nil
	}
	rest := content[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	delimiterLength := len("\n---\n")
	if end < 0 && strings.HasSuffix(rest, "\n---") {
		end = len(rest) - len("\n---")
		delimiterLength = len("\n---")
	}
	if end < 0 {
		return "", "", fmt.Errorf("unterminated frontmatter")
	}
	return rest[end+delimiterLength:], rest[:end], nil
}

func parseArguments(input string) []string {
	matched := argumentPattern.FindAllString(input, -1)
	args := make([]string, len(matched))
	for i, arg := range matched {
		args[i] = strings.Trim(arg, "\"'")
	}
	return args
}

var argumentPattern = regexp.MustCompile(`(?:"[^"]*"|'[^']*'|[^\s"']+)`)
