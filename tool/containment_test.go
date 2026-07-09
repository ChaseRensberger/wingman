package tool

import (
	"context"
	"strings"
	"testing"
)

func TestGrepRejectsAbsolutePathOutsideWorkDir(t *testing.T) {
	workDir := t.TempDir()
	_, err := NewGrepTool().Execute(context.Background(), map[string]any{
		"pattern": "root",
		"path":    "/etc",
	}, workDir)
	if err == nil || !strings.Contains(err.Error(), "path escapes working directory") {
		t.Fatalf("err = %v, want path escape", err)
	}
}

func TestGlobRejectsAbsolutePathOutsideWorkDir(t *testing.T) {
	workDir := t.TempDir()
	_, err := NewGlobTool().Execute(context.Background(), map[string]any{
		"pattern": "*",
		"path":    "/etc",
	}, workDir)
	if err == nil || !strings.Contains(err.Error(), "path escapes working directory") {
		t.Fatalf("err = %v, want path escape", err)
	}
}
