package server

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/store"
)

func TestResolveInstructionsOrdersAndDescribesSources(t *testing.T) {
	root := t.TempDir()
	globalPath := filepath.Join(root, "config", agentsFileName)
	workDir := filepath.Join(root, "project")
	projectPath := filepath.Join(workDir, agentsFileName)
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(globalPath, []byte("Use global rules."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectPath, []byte("Use project rules."), 0o644); err != nil {
		t.Fatal(err)
	}

	server := New(Config{GlobalInstructionsPath: globalPath})
	resolved, sources, err := server.resolveInstructions(&store.Agent{Instructions: "Agent rules."}, workDir)
	if err != nil {
		t.Fatal(err)
	}
	dateAt := strings.Index(resolved, "Current date:")
	globalAt := strings.Index(resolved, "Use global rules.")
	projectAt := strings.Index(resolved, "Use project rules.")
	if !strings.HasPrefix(resolved, "Agent rules.") || dateAt < 0 || globalAt <= dateAt || projectAt <= globalAt {
		t.Fatalf("resolved instructions = %q", resolved)
	}
	if len(sources) != 2 || sources[0].Kind != "global" || sources[0].Order != 1 || sources[1].Kind != "project" || sources[1].Order != 2 {
		t.Fatalf("sources = %#v", sources)
	}
	wantHash := sha256.Sum256([]byte("Use global rules."))
	canonicalGlobalPath, err := filepath.EvalSymlinks(globalPath)
	if err != nil {
		t.Fatal(err)
	}
	if sources[0].Path != canonicalGlobalPath || sources[0].SHA256 != fmt.Sprintf("%x", wantHash) || sources[0].ResolvedAt.IsZero() {
		t.Fatalf("global source = %#v", sources[0])
	}
}

func TestResolveInstructionsAllowsAbsentFiles(t *testing.T) {
	server := New(Config{GlobalInstructionsPath: filepath.Join(t.TempDir(), agentsFileName)})
	agent := &store.Agent{Instructions: "Agent rules."}
	resolved, sources, err := server.resolveInstructions(agent, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resolved, agent.Instructions+"\n\nCurrent date:") || len(sources) != 0 {
		t.Fatalf("resolved = %#v, sources = %#v", resolved, sources)
	}
}

func TestResolveInstructionsReportsReadErrors(t *testing.T) {
	workDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(workDir, agentsFileName), 0o755); err != nil {
		t.Fatal(err)
	}
	server := New(Config{})
	_, _, err := server.resolveInstructions(&store.Agent{}, workDir)
	if err == nil || !strings.Contains(err.Error(), "read project instructions") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveSkillsProjectOverridesAndHonorsPermission(t *testing.T) {
	global, workDir := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(global, "review.md"), []byte("---\ndescription: Global review\n---\nglobal"), 0o644); err != nil {
		t.Fatal(err)
	}
	projectSkills := filepath.Join(workDir, ".wingman", "skills", "review")
	if err := os.MkdirAll(projectSkills, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectSkills, "SKILL.md"), []byte("---\ndescription: Project review\n---\nproject"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := New(Config{GlobalSkillDirs: []string{global}, Permissions: permission.Ruleset{{Action: "skill", Resource: "review", Effect: permission.EffectDeny}}})
	skills, catalog, err := server.resolveSkills(&store.Agent{}, workDir)
	if err != nil || len(skills) != 1 || skills[0].Content != "project" || catalog != "" {
		t.Fatalf("skills=%#v catalog=%q err=%v", skills, catalog, err)
	}
}

func TestEphemeralRunResolvesProjectInstructions(t *testing.T) {
	workDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(workDir, agentsFileName), 0o755); err != nil {
		t.Fatal(err)
	}
	server := New(Config{})
	body := fmt.Sprintf(`{"agent":{"name":"test","model_ref":"test/model"},"message":"hello","working_directory":%q}`, workDir)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/run", strings.NewReader(body)))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "read project instructions") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}
