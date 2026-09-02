package skill

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolpkg "github.com/chaserensberger/wingman/tool"
)

func TestDiscoverParsesAndOverridesSkills(t *testing.T) {
	global, project := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(global, "review.md"), []byte("---\nname: Review\ndescription: Review code\n---\nGlobal"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(project, "review")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\ndescription: Project review\n---\nProject"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "guide.txt"), []byte("guide"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0o644); err != nil {
		t.Fatal(err)
	}
	skills, err := Discover(global, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].ID != "review" || skills[0].Description != "Project review" || skills[0].Content != "Project" || len(skills[0].SupportingFiles) != 1 || skills[0].SupportingFiles[0].Path != "guide.txt" || skills[0].SupportingFiles[0].Content != "guide" {
		t.Fatalf("skills = %#v", skills)
	}
}

func TestBuiltinsIncludesWingman(t *testing.T) {
	skills, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].ID != "wingskill" || skills[0].Name != "WingSkill" || skills[0].Description == "" || !strings.Contains(skills[0].Content, "official Wingman documentation") || skills[0].SHA256 == "" || len(skills[0].EmbeddedResources) != 35 || len(skills[0].SupportingFiles) != 0 {
		t.Fatalf("built-in skills = %#v", skills)
	}
	data, err := json.Marshal(skills[0])
	if err != nil || strings.Contains(string(data), "# Quick Start") {
		t.Fatalf("built-in skill snapshot = %s, err=%v", data, err)
	}
}

func TestDiscoverRejectsDuplicateIDsWithinSource(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"one/review", "two/review"} {
		path := filepath.Join(root, dir)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("Review"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Discover(root); err == nil || !strings.Contains(err.Error(), `duplicate skill ID "review"`) {
		t.Fatalf("Discover() error = %v", err)
	}
}

func TestDiscoverFlatSkillHasNoSupportingFilesAndParsesCRLF(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "review.md"), []byte("---\r\ndescription: Review code\r\n---\r\nReview carefully."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unrelated.txt"), []byte("unrelated"), 0o644); err != nil {
		t.Fatal(err)
	}
	skills, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].Description != "Review code" || skills[0].Content != "Review carefully." || len(skills[0].SupportingFiles) != 0 {
		t.Fatalf("skills = %#v", skills)
	}
}

func TestToolReturnsSnapshot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "guide.md"), []byte("Global guidance."), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := Tool([]Skill{{ID: "review", Name: "Review", Content: "Use snapshot.", BaseDir: root, Location: filepath.Join(root, "SKILL.md"), SupportingFiles: []SupportingFile{{Path: "guide.md", Content: "Global guidance."}}}})
	result, err := tool.Execute(context.Background(), toolpkg.Invocation{Input: map[string]any{"id": "review"}})
	if err != nil || !strings.Contains(result.Text, "Use snapshot.") {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	resource, err := tool.Execute(context.Background(), toolpkg.Invocation{Input: map[string]any{"id": "review", "file": "guide.md"}, WorkDir: t.TempDir()})
	if err != nil || !strings.Contains(resource.Text, "Global guidance.") {
		t.Fatalf("resource=%#v err=%v", resource, err)
	}
	if err := os.WriteFile(filepath.Join(root, "guide.md"), []byte("Changed guidance."), 0o644); err != nil {
		t.Fatal(err)
	}
	resource, err = tool.Execute(context.Background(), toolpkg.Invocation{Input: map[string]any{"id": "review", "file": "guide.md"}})
	if err != nil || !strings.Contains(resource.Text, "Global guidance.") || strings.Contains(resource.Text, "Changed guidance.") {
		t.Fatalf("resource after edit=%#v err=%v", resource, err)
	}
	if _, err := tool.Execute(context.Background(), toolpkg.Invocation{Input: map[string]any{"id": "review", "file": "../guide.md"}}); err == nil {
		t.Fatal("skill tool accepted an escaping supporting file")
	}
	check, ok, err := toolpkg.PermissionFor(tool, toolpkg.Invocation{Input: map[string]any{"id": "review"}})
	if err != nil || !ok || check.Action != "skill" || len(check.Resources) != 1 || check.Resources[0] != "review" {
		t.Fatalf("check=%#v ok=%v err=%v", check, ok, err)
	}
}

func TestToolReadsBundledDocumentation(t *testing.T) {
	skills, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	tool := Tool(skills)
	result, err := tool.Execute(context.Background(), toolpkg.Invocation{Input: map[string]any{"id": "wingskill"}})
	if err != nil || !strings.Contains(result.Text, "start-here/quickstart.md") {
		t.Fatalf("skill result=%#v err=%v", result, err)
	}
	page, err := tool.Execute(context.Background(), toolpkg.Invocation{Input: map[string]any{"id": "wingskill", "file": "start-here/quickstart.md"}})
	if err != nil || !strings.Contains(page.Text, "# Quick Start") {
		t.Fatalf("documentation result=%#v err=%v", page, err)
	}
	if _, err := tool.Execute(context.Background(), toolpkg.Invocation{Input: map[string]any{"id": "wingskill", "file": "../go.mod"}}); err == nil {
		t.Fatal("skill tool accepted an escaping documentation path")
	}
}
