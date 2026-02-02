package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wingman/models"
)

type ReadTool struct{}

func NewReadTool() *ReadTool {
	return &ReadTool{}
}

func (t *ReadTool) Name() string {
	return "read"
}

func (t *ReadTool) Description() string {
	return "Read the contents of a file. Returns the file content as text."
}

func (t *ReadTool) Definition() models.WingmanToolDefinition {
	return models.WingmanToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: models.WingmanToolInputSchema{
			Type: "object",
			Properties: map[string]models.WingmanToolProperty{
				"path": {
					Type:        "string",
					Description: "The path to the file to read (relative to working directory or absolute)",
				},
			},
			Required: []string{"path"},
		},
	}
}

func (t *ReadTool) Execute(ctx context.Context, params map[string]any, workDir string) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("path is required")
	}

	if workDir == "" {
		return "", fmt.Errorf("workDir is required for read tool")
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, path)
	}

	path = filepath.Clean(path)

	if !strings.HasPrefix(path, workDir) && !filepath.IsAbs(path) {
		return "", fmt.Errorf("path escapes working directory")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(content), nil
}
