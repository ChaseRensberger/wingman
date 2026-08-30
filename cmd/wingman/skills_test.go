package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallSkill(t *testing.T) {
	root := t.TempDir()
	installed, err := installSkill(context.Background(), "https://github.com/aminblg/simpleenglish", root, fakeSkillGit(t, "---\nname: Simple English\n---\nWrite plainly."))
	if err != nil {
		t.Fatal(err)
	}
	if installed.ID != "simpleenglish" || installed.Revision != strings.Repeat("a", 40) {
		t.Fatalf("installed = %#v", installed)
	}
	if want := filepath.Join(root, "simpleenglish"); installed.Target != want {
		t.Fatalf("target = %q, want %q", installed.Target, want)
	}
	if _, err := os.Stat(filepath.Join(installed.Target, "SKILL.md")); err != nil {
		t.Fatalf("installed skill is missing: %v", err)
	}
}

func TestInstallSkillRejectsExistingTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "simpleenglish"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := installSkill(context.Background(), "https://github.com/aminblg/simpleenglish", root, fakeSkillGit(t, "Skill"))
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("installSkill() error = %v", err)
	}
}

func TestInstallSkillRejectsRepositoryWithoutOneSkill(t *testing.T) {
	root := t.TempDir()
	_, err := installSkill(context.Background(), "https://github.com/aminblg/simpleenglish", root, fakeSkillGit(t, ""))
	if err == nil || !strings.Contains(err.Error(), "exactly one skill") {
		t.Fatalf("installSkill() error = %v", err)
	}
}

func TestInstallFlatSkill(t *testing.T) {
	root := t.TempDir()
	installed, err := installSkill(context.Background(), "https://example.com/review.git", root, fakeSkillGitFile(t, "review.md", "Review carefully."))
	if err != nil {
		t.Fatal(err)
	}
	if installed.ID != "review" {
		t.Fatalf("installed = %#v", installed)
	}
	contents, err := os.ReadFile(filepath.Join(installed.Target, "SKILL.md"))
	if err != nil || string(contents) != "Review carefully." {
		t.Fatalf("installed flat skill = %q, %v", contents, err)
	}
}

func TestSkillRepositoryName(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{source: "https://github.com/aminblg/simpleenglish", want: "simpleenglish"},
		{source: "https://github.com/aminblg/simpleenglish.git", want: "simpleenglish"},
		{source: "http://github.com/aminblg/simpleenglish"},
		{source: "git@github.com:aminblg/simpleenglish.git"},
		{source: "https://github.com/aminblg/simpleenglish?ref=main"},
		{source: "https://github.com"},
	}
	for _, test := range tests {
		got, err := skillRepositoryName(test.source)
		if test.want == "" {
			if err == nil {
				t.Errorf("skillRepositoryName(%q) = %q, want error", test.source, got)
			}
			continue
		}
		if err != nil || got != test.want {
			t.Errorf("skillRepositoryName(%q) = %q, %v; want %q", test.source, got, err, test.want)
		}
	}
}

func fakeSkillGit(t *testing.T, skillFile string) skillGitRunner {
	return fakeSkillGitFile(t, "SKILL.md", skillFile)
}

func fakeSkillGitFile(t *testing.T, name, skillFile string) skillGitRunner {
	t.Helper()
	return func(_ context.Context, dir string, args ...string) ([]byte, error) {
		t.Helper()
		switch args[0] {
		case "clone":
			destination := args[len(args)-1]
			if err := os.MkdirAll(destination, 0o755); err != nil {
				t.Fatal(err)
			}
			if skillFile != "" {
				if err := os.WriteFile(filepath.Join(destination, name), []byte(skillFile), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			return nil, nil
		case "rev-parse":
			if dir == "" {
				t.Fatal("rev-parse did not run in the cloned repository")
			}
			return []byte(strings.Repeat("a", 40) + "\n"), nil
		default:
			return nil, errors.New("unexpected git command")
		}
	}
}
