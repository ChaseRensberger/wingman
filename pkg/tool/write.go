package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"wingman/pkg/models"
)

type WriteTool struct{}

func NewWriteTool() *WriteTool {
	return &WriteTool{}
}

func (t *WriteTool) Name() string {
	return "write"
}

func (t *WriteTool) Description() string {
	return "Write content to a file. Creates the file if it doesn't exist, or overwrites if it does. Creates parent directories as needed."
}

func (t *WriteTool) Definition() models.WingmanToolDefinition {
	return models.WingmanToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: models.WingmanToolInputSchema{
			Type: "object",
			Properties: map[string]models.WingmanToolProperty{
				"path": {
					Type:        "string",
					Description: "The path to the file to write (relative to working directory or absolute)",
				},
				"content": {
					Type:        "string",
					Description: "The content to write to the file",
				},
			},
			Required: []string{"path", "content"},
		},
	}
}

func (t *WriteTool) Execute(ctx context.Context, params map[string]any, workDir string) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("path is required")
	}

	content, ok := params["content"].(string)
	if !ok {
		return "", fmt.Errorf("content is required")
	}

	if workDir == "" {
		return "", fmt.Errorf("workDir is required for write tool")
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, path)
	}

	path = filepath.Clean(path)

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}
