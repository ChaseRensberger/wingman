package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chaserensberger/wingman/core"
)

type EditTool struct{}

func NewEditTool() *EditTool {
	return &EditTool{}
}

func (t *EditTool) Name() string {
	return "edit"
}

func (t *EditTool) Description() string {
	return "Edit an existing file by replacing a specific string with new content. The old_string must match exactly (including whitespace and indentation)."
}

func (t *EditTool) Definition() core.ToolDefinition {
	return core.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: core.ToolInputSchema{
			Type: "object",
			Properties: map[string]core.ToolProperty{
				"path": {
					Type:        "string",
					Description: "The path to the file to edit",
				},
				"old_string": {
					Type:        "string",
					Description: "The exact string to find and replace (must match exactly)",
				},
				"new_string": {
					Type:        "string",
					Description: "The string to replace it with",
				},
			},
			Required: []string{"path", "old_string", "new_string"},
		},
	}
}

func (t *EditTool) Execute(ctx context.Context, params map[string]any, workDir string) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("path is required")
	}

	oldString, ok := params["old_string"].(string)
	if !ok {
		return "", fmt.Errorf("old_string is required")
	}

	newString, ok := params["new_string"].(string)
	if !ok {
		return "", fmt.Errorf("new_string is required")
	}

	if workDir == "" {
		return "", fmt.Errorf("workDir is required for edit tool")
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, path)
	}

	path = filepath.Clean(path)

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	contentStr := string(content)
	count := strings.Count(contentStr, oldString)

	if count == 0 {
		return "", fmt.Errorf("old_string not found in file")
	}

	if count > 1 {
		return "", fmt.Errorf("old_string found %d times, must be unique (add more context)", count)
	}

	newContent := strings.Replace(contentStr, oldString, newString, 1)

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully edited %s", path), nil
}
