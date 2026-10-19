package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/tool"
)

func TestInstructionDiscoveryRecoveryAndHistoricalInputs(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "repo")
	reference := filepath.Join(root, "reference")
	for _, dir := range []string{work, reference} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	guide := filepath.Join(reference, "AGENTS.md")
	file := filepath.Join(reference, "source.go")
	if err := os.WriteFile(guide, []byte("original guidance"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("package example"), 0644); err != nil {
		t.Fatal(err)
	}
	resolvedFile, err := filepath.EvalSymlinks(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(file, filepath.Join(work, "source.go")); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "session.db")
	data, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	sess := &store.Session{ID: "ses_guidance", WorkDir: work}
	if err := data.CreateSession(sess); err != nil {
		t.Fatal(err)
	}
	admission, err := data.AdmitSessionRun(t.Context(), store.SessionRun{ID: "run_guidance", SessionID: sess.ID})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{}
	agent := &store.Agent{Instructions: "agent rules"}
	resolve, err := s.instructionResolver(t.Context(), data, sess, agent, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, initial, err := resolve(t.Context(), agent.Instructions, nil)
	if err != nil || len(initial) != 0 {
		t.Fatalf("nested instructions loaded before read: %#v %v", initial, err)
	}
	result, err := tool.NewReadTool().Execute(t.Context(), tool.Invocation{WorkDir: work, Input: map[string]any{"filePath": "source.go"}})
	if err != nil {
		t.Fatal(err)
	}
	metadata, _ := json.Marshal(result.Metadata)
	use := store.ToolUse{ID: "tlu_read", SessionID: sess.ID, RunID: admission.Run.ID, Step: 1, Name: "read", CallID: "call_read"}
	for _, status := range []string{store.ToolUseStatusProposed, store.ToolUseStatusAuthorized, store.ToolUseStatusStarted, store.ToolUseStatusCompleted} {
		use.Status = status
		if status == store.ToolUseStatusCompleted {
			use.MetadataJSON = metadata
		}
		if err := data.SaveToolUse(t.Context(), use); err != nil {
			t.Fatal(err)
		}
	}
	history := []models.Message{{Role: models.RoleAssistant, Content: models.Content{models.ToolPart{Name: "read", State: models.ToolStateCompleted, ToolUseID: use.ID, CallID: use.CallID, Metadata: result.Metadata}}}}
	system, sources, err := resolve(t.Context(), agent.Instructions, history)
	if err != nil || len(sources) != 1 || sources[0].ToolUseID != use.ID || !strings.Contains(system, "original guidance") {
		t.Fatalf("discovery = %#v %q %v", sources, system, err)
	}
	hash, blobs, err := models.SnapshotCallInput(models.Request{System: system, Instructions: sources})
	if err != nil {
		t.Fatal(err)
	}
	trace, _ := json.Marshal(models.CallTrace{Version: "1", InputHash: hash})
	if err := data.UpsertModelCall(t.Context(), store.ModelCall{ID: "mcl_guidance", SessionID: sess.ID, Trace: trace, InputBlobs: blobs}); err != nil {
		t.Fatal(err)
	}
	if err := data.Close(); err != nil {
		t.Fatal(err)
	}
	data, err = store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	resolve, err = s.instructionResolver(t.Context(), data, sess, agent, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(guide, []byte("updated guidance"), 0644); err != nil {
		t.Fatal(err)
	}
	system, resumed, err := resolve(t.Context(), agent.Instructions, nil)
	if err != nil || len(resumed) != 1 || resumed[0].SHA256 == sources[0].SHA256 || !strings.Contains(system, "updated guidance") {
		t.Fatalf("recovery without transcript: %#v %q %v", resumed, system, err)
	}
	old, err := data.GetModelCallInput(t.Context(), sess.ID, "mcl_guidance")
	if err != nil || old.Instructions[0].Content != "original guidance" || old.Instructions[0].ResolvedPath != resolvedFile || old.Instructions[0].ToolUseID != use.ID {
		t.Fatalf("historical snapshot: %#v %v", old, err)
	}
	if err := os.Remove(guide); err != nil {
		t.Fatal(err)
	}
	system, removed, err := resolve(t.Context(), agent.Instructions, nil)
	if err != nil || len(removed) != 0 || strings.Contains(system, "updated guidance") {
		t.Fatalf("removed guidance still active: %#v %v", removed, err)
	}
}
