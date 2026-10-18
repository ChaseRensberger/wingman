package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePathResources(t *testing.T) {
	root := t.TempDir()
	workDir := filepath.Join(root, "work")
	if err := os.Mkdir(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, path, resource string
	}{
		{"relative internal", "docs/new.md", "docs/new.md"},
		{"absolute internal", filepath.Join(root, "work", "new.md"), "new.md"},
		{"relative external", "../reference/new.md", filepath.ToSlash(filepath.Join(root, "reference", "new.md"))},
		{"absolute external", filepath.Join(root, "reference", "new.md"), filepath.ToSlash(filepath.Join(root, "reference", "new.md"))},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, resource, err := ResolvePath(workDir, test.path)
			if err != nil || !filepath.IsAbs(path) || resource != test.resource {
				t.Fatalf("ResolvePath() = %q, %q, %v; want %q", path, resource, err, test.resource)
			}
		})
	}
}

func TestResolvePathFollowsLinksForPermissions(t *testing.T) {
	workDir := t.TempDir()
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, target string }{
		{"directory", outside},
		{"dangling", filepath.Join(outside, "missing", "file.md")},
	} {
		t.Run(test.name, func(t *testing.T) {
			link := filepath.Join(workDir, test.name)
			if err := os.Symlink(test.target, link); err != nil {
				t.Skip(err)
			}
			path, resource, err := ResolvePath(workDir, link)
			if err != nil || path != link || resource != filepath.ToSlash(test.target) {
				t.Fatalf("ResolvePath() = %q, %q, %v", path, resource, err)
			}
			if test.name == "directory" {
				_, resource, err := ResolvePath(workDir, filepath.Join(link, "missing", "file.md"))
				if err != nil || resource != filepath.ToSlash(filepath.Join(outside, "missing", "file.md")) {
					t.Fatalf("new linked target = %q, %v", resource, err)
				}
			}
		})
	}
}

func TestFileToolsExecuteOutsideWorkDir(t *testing.T) {
	for _, test := range []struct {
		tool  Tool
		input func(string) map[string]any
		want  string
	}{
		{NewReadTool(), func(path string) map[string]any { return map[string]any{"filePath": path} }, "before"},
		{NewWriteTool(), func(path string) map[string]any { return map[string]any{"filePath": path, "content": "after\n"} }, "Successfully wrote"},
		{NewEditTool(), func(path string) map[string]any {
			return map[string]any{"filePath": path, "oldString": "before", "newString": "after"}
		}, ""},
		{NewApplyPatchTool(), func(path string) map[string]any {
			return map[string]any{"patchText": "*** Begin Patch\n*** Update File: " + path + "\n@@\n-before\n+after\n*** End Patch"}
		}, "Success"},
		{NewGrepTool(), func(path string) map[string]any {
			return map[string]any{"path": filepath.Dir(path), "pattern": "before"}
		}, "before"},
		{NewGlobTool(), func(path string) map[string]any { return map[string]any{"path": filepath.Dir(path), "pattern": "*.md"} }, "file.md"},
	} {
		t.Run(test.tool.Name(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "file.md")
			if err := os.WriteFile(path, []byte("before\n"), 0644); err != nil {
				t.Fatal(err)
			}
			result, err := test.tool.Execute(context.Background(), Invocation{WorkDir: t.TempDir(), Input: test.input(path)})
			if err != nil || !strings.Contains(result.Text, test.want) {
				t.Fatalf("Execute() = %#v, %v", result, err)
			}
		})
	}
}

func TestGlobRequiresDirectoryInPath(t *testing.T) {
	for _, pattern := range []string{"../*.md", "nested/../../*.md", filepath.Join(t.TempDir(), "*.md")} {
		_, err := NewGlobTool().Execute(context.Background(), Invocation{WorkDir: t.TempDir(), Input: map[string]any{"pattern": pattern}})
		if err == nil || !strings.Contains(err.Error(), "use path") {
			t.Fatalf("pattern %q error = %v", pattern, err)
		}
	}
}

func TestSearchDoesNotFollowImplicitSymlinks(t *testing.T) {
	workDir, outside := t.TempDir(), t.TempDir()
	path := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(path, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(outside, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "nested", "secret.md"), []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, link := range []struct{ name, target string }{{"file.md", path}, {"directory", outside}} {
		if err := os.Symlink(link.target, filepath.Join(workDir, link.name)); err != nil {
			t.Skip(err)
		}
	}
	grep, err := NewGrepTool().Execute(context.Background(), Invocation{WorkDir: workDir, Input: map[string]any{"pattern": "secret"}})
	if err != nil || !strings.HasPrefix(grep.Text, "No matches") {
		t.Fatalf("grep = %#v, %v", grep, err)
	}
	glob, err := NewGlobTool().Execute(context.Background(), Invocation{WorkDir: workDir, Input: map[string]any{"pattern": "directory/*.md"}})
	if err != nil || !strings.HasPrefix(glob.Text, "No files") {
		t.Fatalf("glob = %#v, %v", glob, err)
	}
	glob, err = NewGlobTool().Execute(context.Background(), Invocation{WorkDir: workDir, Input: map[string]any{"pattern": "directory/nested/*.md"}})
	if err != nil || !strings.HasPrefix(glob.Text, "No files") {
		t.Fatalf("glob through a linked ancestor = %#v, %v", glob, err)
	}
	glob, err = NewGlobTool().Execute(context.Background(), Invocation{WorkDir: workDir, Input: map[string]any{"path": "directory", "pattern": "*.md"}})
	if err != nil || !strings.Contains(glob.Text, "secret.md") {
		t.Fatalf("explicit glob = %#v, %v", glob, err)
	}
}

func TestPatchDeletesLinkWithoutDeletingTarget(t *testing.T) {
	workDir := t.TempDir()
	target := filepath.Join(t.TempDir(), "target.md")
	if err := os.WriteFile(target, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(workDir, "link.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	_, err := NewApplyPatchTool().Execute(context.Background(), Invocation{WorkDir: workDir, Input: map[string]any{"patchText": "*** Begin Patch\n*** Delete File: link.md\n*** End Patch"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("link still exists: %v", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "keep" {
		t.Fatalf("target = %q, %v", data, err)
	}
}
