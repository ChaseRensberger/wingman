package storage_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/chaserensberger/wingman/plugins/storage"
	"github.com/chaserensberger/wingman/wingagent/loop/looptest"
	"github.com/chaserensberger/wingman/wingagent/session"
	"github.com/chaserensberger/wingman/storage"
	"github.com/chaserensberger/wingman/wingmodels"
)

// TestMigrationsApplyCleanlyAndAreIdempotent asks: does opening a fresh DB
// apply all migrations, and does opening it again do nothing?
func TestMigrationsApplyCleanlyAndAreIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	store, err := storage.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("first open failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open direct connection: %v", err)
	}

	rows, err := db.Query(`SELECT version, name FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	defer rows.Close()

	var migrations []struct {
		version int
		name    string
	}
	for rows.Next() {
		var v int
		var n string
		if err := rows.Scan(&v, &n); err != nil {
			t.Fatalf("scan migration row: %v", err)
		}
		migrations = append(migrations, struct {
			version int
			name    string
		}{v, n})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}

	if len(migrations) < 2 {
		t.Fatalf("expected at least 2 migrations, got %d", len(migrations))
	}
	if migrations[0].version != 1 || migrations[0].name != "init" {
		t.Errorf("expected migration 1 'init', got %d %q", migrations[0].version, migrations[0].name)
	}
	if migrations[1].version != 2 || migrations[1].name != "session_title" {
		t.Errorf("expected migration 2 'session_title', got %d %q", migrations[1].version, migrations[1].name)
	}

	firstCount := len(migrations)

	if err := db.Close(); err != nil {
		t.Fatalf("close direct connection: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	store, err = storage.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("second open failed: %v", err)
	}
	defer store.Close()

	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open direct connection second time: %v", err)
	}
	defer db.Close()

	var secondCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&secondCount); err != nil {
		t.Fatalf("count schema_migrations second time: %v", err)
	}
	if secondCount != firstCount {
		t.Errorf("expected %d migrations after re-open, got %d", firstCount, secondCount)
	}
}

// TestAgentCRUDRoundTrip asks: does Create → Get → Update → List → Delete
// behave correctly for agents, including non-trivial fields?
func TestAgentCRUDRoundTrip(t *testing.T) {
	store := newStore(t)

	agent := &storage.Agent{
		Name:         "test-agent",
		Instructions: "You are a test agent.",
		Tools:        []string{"bash", "read"},
		Provider:     "anthropic",
		Model:        "claude-sonnet",
		Options:      map[string]any{"temperature": 0.7},
		OutputSchema: map[string]any{"type": "object"},
	}

	if err := store.CreateAgent(agent); err != nil {
		t.Fatalf("create agent failed: %v", err)
	}
	if agent.ID == "" {
		t.Fatal("expected agent ID to be set")
	}
	if !strings.HasPrefix(agent.ID, "agt_") {
		t.Errorf("expected ID prefix 'agt_', got %q", agent.ID)
	}

	originalUpdatedAt := agent.UpdatedAt

	got, err := store.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("get agent failed: %v", err)
	}
	if got.Name != agent.Name {
		t.Errorf("name mismatch: got %q, want %q", got.Name, agent.Name)
	}
	if got.Instructions != agent.Instructions {
		t.Errorf("instructions mismatch: got %q, want %q", got.Instructions, agent.Instructions)
	}
	if !reflect.DeepEqual(got.Tools, agent.Tools) {
		t.Errorf("tools mismatch: got %v, want %v", got.Tools, agent.Tools)
	}
	if got.Provider != agent.Provider {
		t.Errorf("provider mismatch: got %q, want %q", got.Provider, agent.Provider)
	}
	if got.Model != agent.Model {
		t.Errorf("model mismatch: got %q, want %q", got.Model, agent.Model)
	}
	if !reflect.DeepEqual(got.Options, agent.Options) {
		t.Errorf("options mismatch: got %v, want %v", got.Options, agent.Options)
	}
	if !reflect.DeepEqual(got.OutputSchema, agent.OutputSchema) {
		t.Errorf("output_schema mismatch: got %v, want %v", got.OutputSchema, agent.OutputSchema)
	}

	time.Sleep(1100 * time.Millisecond)

	agent.Name = "updated-agent"
	if err := store.UpdateAgent(agent); err != nil {
		t.Fatalf("update agent failed: %v", err)
	}

	got, err = store.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("get agent after update failed: %v", err)
	}
	if got.Name != "updated-agent" {
		t.Errorf("expected updated name, got %q", got.Name)
	}
	if got.UpdatedAt == originalUpdatedAt {
		t.Error("expected UpdatedAt to change after update")
	}

	agents, err := store.ListAgents()
	if err != nil {
		t.Fatalf("list agents failed: %v", err)
	}
	found := false
	for _, a := range agents {
		if a.ID == agent.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected agent to be in list")
	}

	if err := store.DeleteAgent(agent.ID); err != nil {
		t.Fatalf("delete agent failed: %v", err)
	}
	if _, err := store.GetAgent(agent.ID); err == nil {
		t.Fatal("expected error getting deleted agent")
	}
	if err := store.DeleteAgent(agent.ID); err == nil {
		t.Fatal("expected error deleting agent twice")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got %v", err)
	}
}

// TestSessionRoundTripWithMessagesAndParts asks: does CreateSession with
// initial history, followed by GetSession, faithfully round-trip messages
// with multiple part types?
func TestSessionRoundTripWithMessagesAndParts(t *testing.T) {
	store := newStore(t)

	history := []wingmodels.Message{
		{
			Role:    wingmodels.RoleUser,
			Content: wingmodels.Content{wingmodels.TextPart{Text: "user says hi"}},
		},
		{
			Role: wingmodels.RoleAssistant,
			Content: wingmodels.Content{
				wingmodels.TextPart{Text: "assistant replies"},
				wingmodels.ToolCallPart{CallID: "call1", Name: "test_tool", Input: map[string]any{"arg": "value"}},
			},
		},
	}

	sess := &storage.Session{
		Title:   "test",
		WorkDir: "/tmp",
		History: history,
	}
	if err := store.CreateSession(sess); err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("expected session ID to be set")
	}
	if !strings.HasPrefix(sess.ID, "ses_") {
		t.Errorf("expected ID prefix 'ses_', got %q", sess.ID)
	}

	got, err := store.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("get session failed: %v", err)
	}
	if len(got.History) != len(history) {
		t.Fatalf("history length mismatch: got %d, want %d", len(got.History), len(history))
	}
	for i, want := range history {
		g := got.History[i]
		if g.Role != want.Role {
			t.Errorf("message %d role mismatch: got %q, want %q", i, g.Role, want.Role)
		}
		if len(g.Content) != len(want.Content) {
			t.Fatalf("message %d content length mismatch: got %d, want %d", i, len(g.Content), len(want.Content))
		}
		for j, wantPart := range want.Content {
			gotPart := g.Content[j]
			if gotPart.Type() != wantPart.Type() {
				t.Errorf("message %d part %d type mismatch: got %q, want %q", i, j, gotPart.Type(), wantPart.Type())
			}
			if !reflect.DeepEqual(gotPart, wantPart) {
				t.Errorf("message %d part %d value mismatch:\ngot  %+v\nwant %+v", i, j, gotPart, wantPart)
			}
		}
	}
}

// TestAppendMessageAssignsNextIdxAndBumpsUpdatedAt asks: does AppendMessage
// compute the next idx correctly and bump the parent session's updated_at?
func TestAppendMessageAssignsNextIdxAndBumpsUpdatedAt(t *testing.T) {
	store := newStore(t)

	sess := &storage.Session{Title: "append-test", WorkDir: "/tmp"}
	if err := store.CreateSession(sess); err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	originalUpdatedAt := sess.UpdatedAt

	time.Sleep(1100 * time.Millisecond)

	msgA := wingmodels.NewUserText("message A")
	msgB := wingmodels.NewUserText("message B")
	if err := store.AppendMessage(sess.ID, msgA); err != nil {
		t.Fatalf("append message A failed: %v", err)
	}
	if err := store.AppendMessage(sess.ID, msgB); err != nil {
		t.Fatalf("append message B failed: %v", err)
	}

	got, err := store.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("get session failed: %v", err)
	}
	if len(got.History) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got.History))
	}
	if got.History[0].Content[0].(wingmodels.TextPart).Text != "message A" {
		t.Errorf("expected first message to be 'message A', got %v", got.History[0].Content)
	}
	if got.History[1].Content[0].(wingmodels.TextPart).Text != "message B" {
		t.Errorf("expected second message to be 'message B', got %v", got.History[1].Content)
	}
	if got.UpdatedAt <= originalUpdatedAt {
		t.Errorf("expected UpdatedAt to increase: original %q, got %q", originalUpdatedAt, got.UpdatedAt)
	}

	if err := store.AppendMessage("ses_nonexistent", wingmodels.NewUserText("nope")); err == nil {
		t.Fatal("expected error appending to non-existent session")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got %v", err)
	}
}

// TestReplaceMessagesAtomicallyRewrites asks: does ReplaceMessages clear
// prior history and write the new slice atomically?
func TestReplaceMessagesAtomicallyRewrites(t *testing.T) {
	store := newStore(t)

	sess := &storage.Session{
		Title:   "replace-test",
		WorkDir: "/tmp",
		History: []wingmodels.Message{
			wingmodels.NewUserText("one"),
			wingmodels.NewUserText("two"),
			wingmodels.NewUserText("three"),
		},
	}
	if err := store.CreateSession(sess); err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	newMsgs := []wingmodels.Message{
		wingmodels.NewUserText("new-one"),
		wingmodels.NewAssistantText("new-two"),
	}
	if err := store.ReplaceMessages(sess.ID, newMsgs); err != nil {
		t.Fatalf("replace messages failed: %v", err)
	}

	got, err := store.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("get session failed: %v", err)
	}
	if len(got.History) != 2 {
		t.Fatalf("expected 2 messages after replace, got %d", len(got.History))
	}
	if got.History[0].Content[0].(wingmodels.TextPart).Text != "new-one" {
		t.Errorf("expected first replaced message 'new-one', got %v", got.History[0].Content)
	}
	if got.History[1].Content[0].(wingmodels.TextPart).Text != "new-two" {
		t.Errorf("expected second replaced message 'new-two', got %v", got.History[1].Content)
	}

	if err := store.ReplaceMessages("ses_nonexistent", newMsgs); err == nil {
		t.Fatal("expected error replacing non-existent session")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got %v", err)
	}
}

// TestDeleteSessionCascadesToMessagesAndParts asks: does ON DELETE CASCADE
// actually remove messages and parts when a session is deleted?
func TestDeleteSessionCascadesToMessagesAndParts(t *testing.T) {
	dbPath, store := newStoreWithPath(t)

	sess := &storage.Session{
		Title:   "cascade-test",
		WorkDir: "/tmp",
		History: []wingmodels.Message{
			wingmodels.NewUserText("msg1"),
			wingmodels.NewUserText("msg2"),
		},
	}
	if err := store.CreateSession(sess); err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	// Append messages with multiple parts each.
	msgA := wingmodels.Message{
		Role: wingmodels.RoleUser,
		Content: wingmodels.Content{
			wingmodels.TextPart{Text: "part1"},
			wingmodels.TextPart{Text: "part2"},
		},
	}
	msgB := wingmodels.Message{
		Role: wingmodels.RoleAssistant,
		Content: wingmodels.Content{
			wingmodels.TextPart{Text: "part3"},
			wingmodels.ToolCallPart{CallID: "c1", Name: "tool", Input: map[string]any{}},
		},
	}
	if err := store.AppendMessage(sess.ID, msgA); err != nil {
		t.Fatalf("append msgA failed: %v", err)
	}
	if err := store.AppendMessage(sess.ID, msgB); err != nil {
		t.Fatalf("append msgB failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open direct connection: %v", err)
	}
	defer db.Close()

	var msgCountBefore int
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, sess.ID).Scan(&msgCountBefore); err != nil {
		t.Fatalf("count messages before delete: %v", err)
	}
	if msgCountBefore == 0 {
		t.Fatal("expected messages before delete")
	}

	var partCountBefore int
	if err := db.QueryRow(`SELECT COUNT(*) FROM parts WHERE message_id IN (SELECT id FROM messages WHERE session_id = ?)`, sess.ID).Scan(&partCountBefore); err != nil {
		t.Fatalf("count parts before delete: %v", err)
	}
	if partCountBefore == 0 {
		t.Fatal("expected parts before delete")
	}

	if err := store.DeleteSession(sess.ID); err != nil {
		t.Fatalf("delete session failed: %v", err)
	}

	var msgCountAfter int
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, sess.ID).Scan(&msgCountAfter); err != nil {
		t.Fatalf("count messages after delete: %v", err)
	}
	if msgCountAfter != 0 {
		t.Errorf("expected 0 messages after delete, got %d", msgCountAfter)
	}

	var partCountAfter int
	if err := db.QueryRow(`SELECT COUNT(*) FROM parts WHERE message_id IN (SELECT id FROM messages WHERE session_id = ?)`, sess.ID).Scan(&partCountAfter); err != nil {
		t.Fatalf("count parts after delete: %v", err)
	}
	if partCountAfter != 0 {
		t.Errorf("expected 0 parts after delete, got %d", partCountAfter)
	}
}

// TestStoragePluginLoadsHistoryAndPersistsNewMessages asks: does the storage
// Plugin load existing history into the loop and persist new messages back?
func TestStoragePluginLoadsHistoryAndPersistsNewMessages(t *testing.T) {
	store := newStore(t)

	// Pre-seed a session with one user message.
	sessionID := createSessionWithHistory(t, store, []wingmodels.Message{
		wingmodels.NewUserText("hello from before"),
	})

	model := looptest.NewRecordingModel(looptest.Reply("response from model"))

	sess := session.New(
		session.WithModel(model),
		session.WithPlugin(storageplugin.NewPlugin(store, sessionID)),
	)

	ctx := context.Background()
	_, err := sess.Run(ctx, "hello now")
	if err != nil {
		t.Fatalf("session run failed: %v", err)
	}

	// Verify the model saw the pre-existing history at the start.
	reqs := model.Requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 model request, got %d", len(reqs))
	}
	msgs := reqs[0].Messages
	if len(msgs) < 1 {
		t.Fatalf("expected at least 1 message in request, got %d", len(msgs))
	}
	first, ok := msgs[0].Content[0].(wingmodels.TextPart)
	if !ok || first.Text != "hello from before" {
		t.Errorf("expected first request message to be 'hello from before', got %+v", msgs[0].Content)
	}

	// Verify the persisted history contains the pre-existing message and
	// the assistant reply. The user message added by Run is not currently
	// auto-persisted by the storage plugin (only loop MessageEvents are).
	stored, err := store.GetSession(sessionID)
	if err != nil {
		t.Fatalf("get session after run: %v", err)
	}
	if len(stored.History) < 2 {
		t.Fatalf("expected at least 2 messages in storage, got %d", len(stored.History))
	}

	firstStored, ok := stored.History[0].Content[0].(wingmodels.TextPart)
	if !ok || firstStored.Text != "hello from before" {
		t.Errorf("expected first stored message to be 'hello from before', got %+v", stored.History[0].Content)
	}

	lastIdx := len(stored.History) - 1
	lastStored, ok := stored.History[lastIdx].Content[0].(wingmodels.TextPart)
	if !ok || lastStored.Text != "response from model" {
		t.Errorf("expected last stored message to be 'response from model', got %+v", stored.History[lastIdx].Content)
	}
}

func createSessionWithHistory(t *testing.T, store storage.Store, history []wingmodels.Message) string {
	t.Helper()
	sess := &storage.Session{
		Title:   "plugin-test",
		WorkDir: "/tmp",
		History: history,
	}
	if err := store.CreateSession(sess); err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	return sess.ID
}
