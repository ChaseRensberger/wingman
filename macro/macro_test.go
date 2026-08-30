package macro

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverAndExpand(t *testing.T) {
	workDir := t.TempDir()
	path := filepath.Join(workDir, ".wingman", "macros", "review", "security.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\ndescription: Review security\nagent: reviewer\nmodel: test/model\n---\nReview $1. Focus on $2."), 0o644); err != nil {
		t.Fatal(err)
	}

	macros, err := Discover(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(macros) != 1 || macros[0].ID != "review/security" || macros[0].AgentID != "reviewer" || macros[0].ModelRef != "test/model" {
		t.Fatalf("macros = %#v", macros)
	}
	if got := Expand(macros[0], `"auth middleware" missing tests`); got != "Review auth middleware. Focus on missing tests." {
		t.Fatalf("expanded = %q", got)
	}
}

func TestExpandAppendsArgumentsWithoutPlaceholder(t *testing.T) {
	got := Expand(Macro{Template: "Review this change."}, "Focus on tests.")
	if got != "Review this change.\n\nFocus on tests." {
		t.Fatalf("expanded = %q", got)
	}
}

func TestDiscoverRejectsInvalidMacros(t *testing.T) {
	workDir := t.TempDir()
	path := filepath.Join(workDir, ".wingman", "macros", "review.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\nunknown: value\n---\nReview"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(workDir); err == nil || !strings.Contains(err.Error(), "field unknown not found") {
		t.Fatalf("Discover() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("Review !`git diff`"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(workDir); err == nil || !strings.Contains(err.Error(), "shell interpolation") {
		t.Fatalf("Discover() error = %v", err)
	}
}
