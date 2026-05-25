package tool

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type EditTool struct{}

func NewEditTool() *EditTool { return &EditTool{} }

func (t *EditTool) Name() string { return "edit" }

func (t *EditTool) Description() string {
	return "Edit an existing file by replacing oldString with newString. Returns diff metadata for UI rendering."
}

func (t *EditTool) Definition() Definition {
	return Definition{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"filePath": {
					Type:        "string",
					Description: "The path to the file to edit",
				},
				"oldString": {
					Type:        "string",
					Description: "The exact string to find and replace",
				},
				"newString": {
					Type:        "string",
					Description: "The string to replace oldString with",
				},
				"replaceAll": {
					Type:        "boolean",
					Description: "Replace all occurrences of oldString instead of requiring a unique match",
				},
			},
			Required: []string{"filePath", "oldString", "newString"},
		},
	}
}

func (t *EditTool) DirectoryScoped() {}

func (t *EditTool) Execute(ctx context.Context, params map[string]any, workDir string) (Result, error) {
	filePath, ok := params["filePath"].(string)
	if !ok || filePath == "" {
		return Result{}, fmt.Errorf("filePath is required")
	}
	oldString, ok := params["oldString"].(string)
	if !ok {
		return Result{}, fmt.Errorf("oldString is required")
	}
	newString, ok := params["newString"].(string)
	if !ok {
		return Result{}, fmt.Errorf("newString is required")
	}
	if oldString == newString {
		return Result{}, fmt.Errorf("oldString and newString are identical")
	}
	if workDir == "" {
		return Result{}, fmt.Errorf("workDir is required for edit tool")
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

	content, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("failed to read file: %w", err)
	}
	oldContent := string(content)
	count := strings.Count(oldContent, oldString)
	if count == 0 {
		return Result{}, fmt.Errorf("oldString not found in file")
	}
	replaceAll, _ := params["replaceAll"].(bool)
	if count > 1 && !replaceAll {
		return Result{}, fmt.Errorf("oldString found %d times, must be unique or replaceAll must be true", count)
	}
	replacements := 1
	if replaceAll {
		replacements = -1
	}
	newContent := strings.Replace(oldContent, oldString, newString, replacements)
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return Result{}, fmt.Errorf("failed to write file: %w", err)
	}

	patch, additions, deletions := unifiedPatch(rel, oldContent, newContent)
	return Result{
		Text:     fmt.Sprintf("Successfully edited %s", path),
		Metadata: fileDiffMetadata(path, rel, "update", patch, additions, deletions),
	}, nil
}
