package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type WriteTool struct{}

func NewWriteTool() *WriteTool { return &WriteTool{} }

func (t *WriteTool) Name() string { return "write" }

func (t *WriteTool) Description() string {
	return "Write content to a file. Creates parent directories as needed and returns diff metadata for UI rendering."
}

func (t *WriteTool) Definition() Definition {
	return Definition{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"filePath": {
					Type:        "string",
					Description: "The path to the file to write (relative to working directory or absolute)",
				},
				"content": {
					Type:        "string",
					Description: "The content to write to the file",
				},
			},
			Required: []string{"filePath", "content"},
		},
	}
}

func (t *WriteTool) DirectoryScoped() {}

func (t *WriteTool) Execute(ctx context.Context, params map[string]any, workDir string) (Result, error) {
	filePath, ok := params["filePath"].(string)
	if !ok || filePath == "" {
		return Result{}, fmt.Errorf("filePath is required")
	}
	content, ok := params["content"].(string)
	if !ok {
		return Result{}, fmt.Errorf("content is required")
	}
	if workDir == "" {
		return Result{}, fmt.Errorf("workDir is required for write tool")
	}

	path, rel, err := resolveWorkPath(workDir, filePath)
	if err != nil {
		return Result{}, err
	}
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	default:
	}

	oldBytes, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("failed to read existing file: %w", err)
	}
	oldContent := string(oldBytes)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return Result{}, fmt.Errorf("failed to create directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return Result{}, fmt.Errorf("failed to write file: %w", err)
	}

	patch, additions, deletions := unifiedPatch(rel, oldContent, content)
	kind := "update"
	if oldBytes == nil {
		kind = "add"
	}
	return Result{
		Text:     fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path),
		Metadata: fileDiffMetadata(path, rel, kind, patch, additions, deletions),
	}, nil
}
