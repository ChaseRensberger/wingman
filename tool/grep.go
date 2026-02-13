package tool

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chaserensberger/wingman/models"
)

type GrepTool struct{}

func NewGrepTool() *GrepTool {
	return &GrepTool{}
}

func (t *GrepTool) Name() string {
	return "grep"
}

func (t *GrepTool) Description() string {
	return "Search for a pattern in files. Returns matching lines with file paths and line numbers."
}

func (t *GrepTool) Definition() models.WingmanToolDefinition {
	return models.WingmanToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: models.WingmanToolInputSchema{
			Type: "object",
			Properties: map[string]models.WingmanToolProperty{
				"pattern": {
					Type:        "string",
					Description: "The regex pattern to search for",
				},
				"path": {
					Type:        "string",
					Description: "File or directory to search in (optional, defaults to working directory)",
				},
				"include": {
					Type:        "string",
					Description: "File pattern to include (e.g., '*.go', '*.ts')",
				},
			},
			Required: []string{"pattern"},
		},
	}
}

type grepMatch struct {
	File    string
	Line    int
	Content string
}

func (t *GrepTool) Execute(ctx context.Context, params map[string]any, workDir string) (string, error) {
	pattern, ok := params["pattern"].(string)
	if !ok || pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}

	if workDir == "" {
		return "", fmt.Errorf("workDir is required for grep tool")
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex pattern: %w", err)
	}

	searchPath := workDir
	if path, ok := params["path"].(string); ok && path != "" {
		if filepath.IsAbs(path) {
			searchPath = path
		} else {
			searchPath = filepath.Join(workDir, path)
		}
	}

	includePattern := ""
	if include, ok := params["include"].(string); ok {
		includePattern = include
	}

	var matches []grepMatch

	info, err := os.Stat(searchPath)
	if err != nil {
		return "", fmt.Errorf("path not found: %w", err)
	}

	if info.IsDir() {
		err = filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if shouldSkipDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}

			if includePattern != "" {
				matched, _ := filepath.Match(includePattern, d.Name())
				if !matched {
					return nil
				}
			}

			if !isTextFile(d.Name()) {
				return nil
			}

			fileMatches, err := searchFile(path, re)
			if err != nil {
				return nil
			}

			relPath, _ := filepath.Rel(workDir, path)
			for i := range fileMatches {
				fileMatches[i].File = relPath
			}
			matches = append(matches, fileMatches...)
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("failed to walk directory: %w", err)
		}
	} else {
		fileMatches, err := searchFile(searchPath, re)
		if err != nil {
			return "", fmt.Errorf("failed to search file: %w", err)
		}
		relPath, _ := filepath.Rel(workDir, searchPath)
		for i := range fileMatches {
			fileMatches[i].File = relPath
		}
		matches = fileMatches
	}

	if len(matches) == 0 {
		return "No matches found for pattern: " + pattern, nil
	}

	var result strings.Builder
	for _, m := range matches {
		result.WriteString(fmt.Sprintf("%s:%d: %s\n", m.File, m.Line, strings.TrimSpace(m.Content)))
	}

	return result.String(), nil
}

func searchFile(path string, re *regexp.Regexp) ([]grepMatch, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var matches []grepMatch
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			matches = append(matches, grepMatch{
				Line:    lineNum,
				Content: line,
			})
		}
	}

	return matches, scanner.Err()
}

func shouldSkipDir(name string) bool {
	skipDirs := []string{".git", "node_modules", "vendor", ".idea", ".vscode", "__pycache__", ".next", "dist", "build"}
	for _, skip := range skipDirs {
		if name == skip {
			return true
		}
	}
	return false
}

func isTextFile(name string) bool {
	textExtensions := []string{
		".go", ".js", ".ts", ".jsx", ".tsx", ".py", ".rb", ".java", ".c", ".cpp", ".h",
		".rs", ".swift", ".kt", ".scala", ".php", ".pl", ".pm", ".sh", ".bash", ".zsh",
		".json", ".yaml", ".yml", ".toml", ".xml", ".html", ".css", ".scss", ".less",
		".md", ".txt", ".rst", ".org", ".tex", ".sql", ".graphql", ".proto",
		".dockerfile", ".env", ".gitignore", ".editorconfig", "Makefile", "Dockerfile",
	}
	ext := strings.ToLower(filepath.Ext(name))
	for _, textExt := range textExtensions {
		if ext == textExt {
			return true
		}
	}
	baseName := strings.ToLower(filepath.Base(name))
	for _, textExt := range textExtensions {
		if baseName == strings.TrimPrefix(textExt, ".") {
			return true
		}
	}
	return false
}
