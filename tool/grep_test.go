package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGrepKeepsFilenamesWithRelativeWorkDir(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "nested", "example.go")
	if err := os.WriteFile(file, []byte("needle\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	workDir, err := filepath.Rel(cwd, root)
	if err != nil {
		t.Skip("temporary directory is on a different volume")
	}
	for _, test := range []struct{ name, path string }{
		{"default path", ""},
		{"explicit current directory", "."},
		{"relative subdirectory", "nested"},
		{"absolute file", file},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := map[string]any{"pattern": "needle"}
			if test.path != "" {
				input["path"] = test.path
			}
			result, err := NewGrepTool().Execute(t.Context(), Invocation{WorkDir: workDir, Input: input})
			want := filepath.Join("nested", "example.go") + ":1: needle\n"
			if err != nil || result.Text != want {
				t.Fatalf("Execute() = %q, %v; want %q", result.Text, err, want)
			}
		})
	}
}

func TestGrepKeepsOutsideFilenames(t *testing.T) {
	workDir := t.TempDir()
	outside := t.TempDir()
	file := filepath.Join(outside, "example.go")
	if err := os.WriteFile(file, []byte("needle\n"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := NewGrepTool().Execute(context.Background(), Invocation{WorkDir: workDir, Input: map[string]any{"path": outside, "pattern": "needle"}})
	relative, relErr := filepath.Rel(workDir, file)
	if relErr != nil {
		relative = file
	}
	if err != nil || result.Text != relative+":1: needle\n" {
		t.Fatalf("Execute() = %q, %v; want filename %q", result.Text, err, relative)
	}
}
