package tool

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

func (t *GrepTool) Definition() Definition {
	return Definition{
		ReplaySafe:  true,
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
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

func (t *GrepTool) DirectoryScoped() {}

func (t *GrepTool) Execute(ctx context.Context, inv Invocation) (Result, error) {
	pattern, ok := inv.Input["pattern"].(string)
	if !ok || pattern == "" {
		return Result{}, fmt.Errorf("pattern is required")
	}

	if inv.WorkDir == "" {
		return Result{}, fmt.Errorf("workDir is required for grep tool")
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return Result{}, fmt.Errorf("invalid regex pattern: %w", err)
	}

	searchPath := inv.WorkDir
	if path, ok := inv.Input["path"].(string); ok && path != "" {
		resolved, _, err := resolveWorkPath(inv.WorkDir, path)
		if err != nil {
			return Result{}, err
		}
		searchPath = resolved
	}

	includePattern := ""
	if include, ok := inv.Input["include"].(string); ok {
		includePattern = include
	}

	output := &outputCollector{inv: inv}
	matches := 0
	search := func(path string) error {
		rel, _ := filepath.Rel(inv.WorkDir, path)
		return searchFile(ctx, path, re, func(line int, text string) error {
			matches++
			_, err := fmt.Fprintf(output, "%s:%d: %s\n", rel, line, strings.TrimSpace(text))
			return err
		})
	}

	info, err := os.Stat(searchPath)
	if err != nil {
		return Result{}, fmt.Errorf("path not found: %w", err)
	}

	if info.IsDir() {
		err = walkFiles(ctx, searchPath, shouldSkipDir, func(path string) error {
			if includePattern != "" {
				matched, _ := filepath.Match(includePattern, filepath.Base(path))
				if !matched {
					return nil
				}
			}

			if !isTextFile(filepath.Base(path)) {
				return nil
			}

			err := search(path)
			// Tree searches skip unreadable files, but must surface output and cancellation failures.
			if output.err == nil && ctx.Err() == nil {
				return nil
			}
			return err
		})
	} else {
		err = search(searchPath)
	}
	result, outputErr := output.finish(map[string]any{"matches": matches})
	if err != nil {
		return result, err
	}
	if outputErr != nil {
		return result, outputErr
	}
	if matches == 0 {
		return Result{Text: "No matches found for pattern: " + pattern, Metadata: map[string]any{"matches": 0}}, nil
	}

	return result, nil
}

func searchFile(ctx context.Context, path string, re *regexp.Regexp, match func(int, string) error) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			if err := match(lineNum, line); err != nil {
				return err
			}
		}
	}

	return scanner.Err()
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
