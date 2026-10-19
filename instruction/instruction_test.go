package instruction

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoveryOrderSymlinksAndLiveUpdates(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(root, "repo")
	nested := filepath.Join(work, "src")
	external := filepath.Join(root, "external")
	for _, dir := range []string{nested, external} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	global := filepath.Join(root, "global.md")
	for path, text := range map[string]string{global: "global", filepath.Join(root, "AGENTS.md"): "ancestor", filepath.Join(work, "AGENTS.md"): "project", filepath.Join(nested, "AGENTS.md"): "nested", filepath.Join(external, "AGENTS.md"): "external"} {
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	before, err := Discover(t.Context(), global, work, nil)
	if err != nil || len(before) != 3 {
		t.Fatalf("initial = %#v, %v", before, err)
	}
	if before[0].Content != "global" || before[1].Content != "ancestor" || before[2].Content != "project" {
		t.Fatalf("order = %#v", before)
	}
	reads := []Read{{RequestedPath: "src/file.go", ResolvedPath: filepath.Join(nested, "file.go"), ToolUseID: "read_nested"}, {RequestedPath: "linked.md", ResolvedPath: filepath.Join(external, "notes.md"), ToolUseID: "read_link"}}
	after, err := Discover(t.Context(), global, work, reads)
	if err != nil || len(after) != 5 || after[4].ToolUseID != "read_link" {
		t.Fatalf("discovery = %#v, %v", after, err)
	}
	if err := os.WriteFile(filepath.Join(nested, "AGENTS.md"), []byte("updated"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(external, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	updated, err := Discover(t.Context(), global, work, reads)
	if err != nil || len(updated) != 4 || updated[3].Content != "updated" || updated[3].SHA256 == after[3].SHA256 {
		t.Fatalf("update = %#v, %v", updated, err)
	}
	if after[3].Content != "nested" {
		t.Fatal("old snapshot changed")
	}
	if err := os.Symlink(global, filepath.Join(external, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	dedup, err := Discover(t.Context(), global, work, reads)
	if err != nil || len(dedup) != 4 {
		t.Fatalf("dedup = %#v, %v", dedup, err)
	}
	canonicalGlobal, err := filepath.EvalSymlinks(global)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(Render("base", dedup), "Source: "+canonicalGlobal) != 1 {
		t.Fatal("duplicate global instruction")
	}
}
