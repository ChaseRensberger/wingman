package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wingman/models"
)

type ReadTool struct {
	workDir string
}

func NewReadTool(workDir string) *ReadTool {
	return &ReadTool{workDir: workDir}
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

func (t *ReadTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("path is required")
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(t.workDir, path)
	}

	path = filepath.Clean(path)

	if !strings.HasPrefix(path, t.workDir) && !filepath.IsAbs(path) {
		return "", fmt.Errorf("path escapes working directory")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(content), nil
}
