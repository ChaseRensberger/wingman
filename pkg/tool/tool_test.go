package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBashTool(t *testing.T) {
	bash := NewBashTool("/tmp")

	t.Run("executes simple command", func(t *testing.T) {
		output, err := bash.Execute(context.Background(), map[string]any{
			"command": "echo hello",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if output != "hello\n" {
			t.Errorf("expected 'hello\\n', got %q", output)
		}
	})

	t.Run("returns error for missing command", func(t *testing.T) {
		_, err := bash.Execute(context.Background(), map[string]any{})
		if err == nil {
			t.Error("expected error for missing command")
		}
	})

	t.Run("captures stderr", func(t *testing.T) {
		output, _ := bash.Execute(context.Background(), map[string]any{
			"command": "echo error >&2",
		})
		if output != "error\n" {
			t.Errorf("expected stderr output, got %q", output)
		}
	})
}

func TestReadTool(t *testing.T) {
	tmpDir := t.TempDir()
	read := NewReadTool(tmpDir)

	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("reads file content", func(t *testing.T) {
		output, err := read.Execute(context.Background(), map[string]any{
			"path": "test.txt",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if output != "test content" {
			t.Errorf("expected 'test content', got %q", output)
		}
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := read.Execute(context.Background(), map[string]any{
			"path": "nonexistent.txt",
		})
		if err == nil {
			t.Error("expected error for missing file")
		}
	})
}

func TestWriteTool(t *testing.T) {
	tmpDir := t.TempDir()
	write := NewWriteTool(tmpDir)

	t.Run("creates new file", func(t *testing.T) {
		_, err := write.Execute(context.Background(), map[string]any{
			"path":    "new.txt",
			"content": "new content",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, err := os.ReadFile(filepath.Join(tmpDir, "new.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "new content" {
			t.Errorf("expected 'new content', got %q", string(content))
		}
	})

	t.Run("creates nested directories", func(t *testing.T) {
		_, err := write.Execute(context.Background(), map[string]any{
			"path":    "nested/dir/file.txt",
			"content": "nested content",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, err := os.ReadFile(filepath.Join(tmpDir, "nested/dir/file.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "nested content" {
			t.Errorf("expected 'nested content', got %q", string(content))
		}
	})
}

func TestEditTool(t *testing.T) {
	tmpDir := t.TempDir()
	edit := NewEditTool(tmpDir)

	testFile := filepath.Join(tmpDir, "edit.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("replaces string", func(t *testing.T) {
		_, err := edit.Execute(context.Background(), map[string]any{
			"path":       "edit.txt",
			"old_string": "world",
			"new_string": "universe",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, err := os.ReadFile(testFile)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "hello universe" {
			t.Errorf("expected 'hello universe', got %q", string(content))
		}
	})

	t.Run("returns error if string not found", func(t *testing.T) {
		_, err := edit.Execute(context.Background(), map[string]any{
			"path":       "edit.txt",
			"old_string": "nonexistent",
			"new_string": "replacement",
		})
		if err == nil {
			t.Error("expected error for string not found")
		}
	})
}

func TestGlobTool(t *testing.T) {
	tmpDir := t.TempDir()
	glob := NewGlobTool(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "file1.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte(""), 0644)
	os.MkdirAll(filepath.Join(tmpDir, "sub"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "sub", "file3.go"), []byte(""), 0644)

	t.Run("matches simple pattern", func(t *testing.T) {
		output, err := glob.Execute(context.Background(), map[string]any{
			"pattern": "*.go",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if output == "" || output == "No files found matching pattern: *.go" {
			t.Error("expected to find .go files")
		}
	})

	t.Run("returns message for no matches", func(t *testing.T) {
		output, err := glob.Execute(context.Background(), map[string]any{
			"pattern": "*.xyz",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if output != "No files found matching pattern: *.xyz" {
			t.Errorf("expected no matches message, got %q", output)
		}
	})
}

func TestGrepTool(t *testing.T) {
	tmpDir := t.TempDir()
	grep := NewGrepTool(tmpDir)

	testFile := filepath.Join(tmpDir, "search.txt")
	os.WriteFile(testFile, []byte("line one\nline two\nthree lines"), 0644)

	t.Run("finds matching lines", func(t *testing.T) {
		output, err := grep.Execute(context.Background(), map[string]any{
			"pattern": "line",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if output == "" || output == "No matches found for pattern: line" {
			t.Error("expected to find matches")
		}
	})

	t.Run("returns message for no matches", func(t *testing.T) {
		output, err := grep.Execute(context.Background(), map[string]any{
			"pattern": "nonexistent",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if output != "No matches found for pattern: nonexistent" {
			t.Errorf("expected no matches message, got %q", output)
		}
	})

	t.Run("returns error for invalid regex", func(t *testing.T) {
		_, err := grep.Execute(context.Background(), map[string]any{
			"pattern": "[invalid",
		})
		if err == nil {
			t.Error("expected error for invalid regex")
		}
	})
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()
	bash := NewBashTool("/tmp")

	t.Run("registers and retrieves tool", func(t *testing.T) {
		registry.Register(bash)
		tool, err := registry.Get("bash")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tool.Name() != "bash" {
			t.Errorf("expected 'bash', got %q", tool.Name())
		}
	})

	t.Run("returns error for unknown tool", func(t *testing.T) {
		_, err := registry.Get("unknown")
		if err == nil {
			t.Error("expected error for unknown tool")
		}
	})

	t.Run("lists all tools", func(t *testing.T) {
		tools := registry.List()
		if len(tools) != 1 {
			t.Errorf("expected 1 tool, got %d", len(tools))
		}
	})
}
