package store

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSQLiteCreateSessionCommitsEventAndProjection(t *testing.T) {
	data := newTestSQLiteStore(t)
	session := &Session{
		ID:      "ses_created",
		Title:   "Created from an event",
		WorkDir: "/tmp/wingman",
	}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	if session.AggregateVersion != 1 {
		t.Fatalf("aggregate version = %d, want 1", session.AggregateVersion)
	}

	events, err := data.ListAggregateEvents(context.Background(), AggregateRef{Type: AggregateSession, ID: session.ID}, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	event := events[0]
	if event.Version != 1 || event.Type != EventSessionCreated || event.SchemaVersion != 1 {
		t.Fatalf("event = %#v", event)
	}
	if event.GlobalSequence == 0 {
		t.Fatal("global sequence was not assigned")
	}

	projected, err := ProjectSession(events)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := data.GetSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projected, stored) {
		t.Fatalf("replayed projection = %#v, stored = %#v", projected, stored)
	}
}

func TestProjectSessionModelCallsRejectsImmutableIdentityChanges(t *testing.T) {
	now := time.Now().UTC()
	created, err := NewSessionCreatedEvent(Session{ID: "ses_call_projection", CreatedAt: Now(), UpdatedAt: Now()})
	if err != nil {
		t.Fatal(err)
	}
	created.Version = 1
	call := ModelCall{ID: "mcl_projection", SessionID: "ses_call_projection", RunID: "run_projection", Step: 1, Attempt: 1, Status: ModelCallStatusStarted, StartedAt: now, CreatedAt: now, UpdatedAt: now}
	first, err := NewSessionModelCallSavedEvent(call)
	if err != nil {
		t.Fatal(err)
	}
	first.Version = 2
	call.Attempt = 2
	call.UpdatedAt = now.Add(time.Second)
	second, err := NewSessionModelCallSavedEvent(call)
	if err != nil {
		t.Fatal(err)
	}
	second.Version = 3
	if _, err := ProjectSessionModelCalls([]AggregateEvent{created, first, second}); err == nil {
		t.Fatal("ProjectSessionModelCalls accepted an immutable attempt change")
	}
}

func TestSQLiteCreateSessionRollsBackEventWhenProjectionFails(t *testing.T) {
	data := newTestSQLiteStore(t)
	if _, err := data.db.Exec(`
		INSERT INTO sessions (id, title, created_at, updated_at, aggregate_version)
		VALUES ('ses_existing', 'legacy row', ?, ?, 0)
	`, Now(), Now()); err != nil {
		t.Fatal(err)
	}

	err := data.CreateSession(&Session{ID: "ses_existing", Title: "conflict"})
	if err == nil {
		t.Fatal("CreateSession succeeded, want projection error")
	}
	events, listErr := data.ListAggregateEvents(context.Background(), AggregateRef{Type: AggregateSession, ID: "ses_existing"}, 0, 100)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(events) != 0 {
		t.Fatalf("events = %d, want rollback to leave 0", len(events))
	}
	stored, getErr := data.GetSession("ses_existing")
	if getErr != nil {
		t.Fatal(getErr)
	}
	if stored.Title != "legacy row" || stored.AggregateVersion != 0 {
		t.Fatalf("stored session = %#v", stored)
	}
}

func TestSQLiteCreateSessionSerializesConcurrentCreation(t *testing.T) {
	data := newTestSQLiteStore(t)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- data.CreateSession(&Session{ID: "ses_concurrent"})
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	var successes, conflicts int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrAggregateVersionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes = %d, conflicts = %d; want 1 each", successes, conflicts)
	}
	events, err := data.ListAggregateEvents(context.Background(), AggregateRef{Type: AggregateSession, ID: "ses_concurrent"}, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Version != 1 {
		t.Fatalf("events = %#v", events)
	}
}

func TestSQLiteCreateSessionSerializesAcrossStoreHandles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wingman.db")
	first, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, data := range []*SQLiteStore{first, second} {
		wg.Add(1)
		go func(data *SQLiteStore) {
			defer wg.Done()
			<-start
			errs <- data.CreateSession(&Session{ID: "ses_multi_handle"})
		}(data)
	}
	close(start)
	wg.Wait()
	close(errs)

	var successes, conflicts int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrAggregateVersionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes = %d, conflicts = %d; want 1 each", successes, conflicts)
	}
}

func TestSQLiteSessionMetadataEventsCommitAndReplay(t *testing.T) {
	data := newTestSQLiteStore(t)
	session := &Session{ID: "ses_metadata", Title: "Before", WorkDir: "/before"}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	renamed, err := data.RenameSession(context.Background(), session.ID, "After", 1)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := data.MoveSession(context.Background(), session.ID, "/after", "", renamed.AggregateVersion)
	if err != nil {
		t.Fatal(err)
	}
	events, err := data.ListAggregateEvents(context.Background(), AggregateRef{Type: AggregateSession, ID: session.ID}, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[1].Type != EventSessionRenamed || events[2].Type != EventSessionMoved {
		t.Fatalf("events = %#v", events)
	}
	projected, err := ProjectSession(events)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := data.GetSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projected, stored) || !reflect.DeepEqual(projected, moved) {
		t.Fatalf("replayed = %#v, stored = %#v, moved = %#v", projected, stored, moved)
	}
}

func TestSQLiteRebuildsAllSessionProjectionsFromAggregateHistory(t *testing.T) {
	data := newTestSQLiteStore(t)
	ctx := context.Background()
	first := &Session{ID: "ses_rebuild_first", Title: "First", WorkDir: "/before"}
	second := &Session{ID: "ses_rebuild_second", Title: "Second"}
	for _, session := range []*Session{first, second} {
		if err := data.CreateSession(session); err != nil {
			t.Fatal(err)
		}
	}
	first, err := data.RenameSession(ctx, first.ID, "Renamed", first.AggregateVersion)
	if err != nil {
		t.Fatal(err)
	}
	first, err = data.MoveSession(ctx, first.ID, "/after", "", first.AggregateVersion)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.AdmitSessionRun(ctx, SessionRun{ID: "run_rebuild_first", SessionID: first.ID, Message: "first"}); err != nil {
		t.Fatal(err)
	}
	if _, err := data.AdmitSessionRun(ctx, SessionRun{ID: "run_rebuild_second", SessionID: second.ID, Message: "second"}); err != nil {
		t.Fatal(err)
	}

	sessions, err := data.ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("session count = %d, want 2", len(sessions))
	}
	for _, stored := range sessions {
		events, err := data.ListAggregateEvents(ctx, AggregateRef{Type: AggregateSession, ID: stored.ID}, 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		projected, err := ProjectSession(events)
		if err != nil {
			t.Fatalf("rebuild %s: %v", stored.ID, err)
		}
		if !reflect.DeepEqual(projected, stored) {
			t.Fatalf("rebuild %s = %#v, stored = %#v", stored.ID, projected, stored)
		}
	}
}

func TestSQLiteRebuildSessionProjectionsRestoresDerivedRows(t *testing.T) {
	data := newTestSQLiteStore(t)
	ctx := context.Background()
	session := &Session{ID: "ses_rebuild_rows", Title: "original"}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	action, err := data.AdmitSessionRun(ctx, SessionRun{
		ID: "run_rebuild_action", SessionID: session.ID, Kind: SessionRunKindAction,
		Action: "example.run", InputJSON: []byte(`{"value":1}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	message := StoredMessage{ID: "msg_rebuild_rows", SessionID: session.ID, Idx: 1, Role: "assistant", Revision: 1, State: "completed", Parts: []StoredPart{{ID: "prt_rebuild_rows", MessageID: "msg_rebuild_rows", Sequence: 0, Kind: "text", PayloadJSON: []byte(`{"text":"original"}`)}}}
	if err := data.SaveMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	events, err := data.ListAggregateEvents(ctx, AggregateRef{Type: AggregateSession, ID: session.ID}, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ProjectSessionAggregate(events)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.db.Exec(`DELETE FROM parts; DELETE FROM messages; UPDATE sessions SET title = 'disturbed', aggregate_version = 0 WHERE id = ?`, session.ID); err != nil {
		t.Fatal(err)
	}
	if err := data.RebuildSessionProjections(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	gotSession, err := data.GetSession(session.ID)
	if err != nil || !reflect.DeepEqual(gotSession, want.Session) {
		t.Fatalf("session = %#v, %v; want %#v", gotSession, err, want.Session)
	}
	gotMessages, err := data.ListMessages(ctx, session.ID)
	if err != nil || !reflect.DeepEqual(gotMessages, want.Messages) {
		t.Fatalf("messages = %#v, %v; want %#v", gotMessages, err, want.Messages)
	}
	gotRun, err := data.GetSessionRun(ctx, session.ID, action.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotRun.Kind != SessionRunKindAction || gotRun.Action != "example.run" || string(gotRun.InputJSON) != `{"value":1}` {
		t.Fatalf("rebuilt action run = %#v", gotRun)
	}
}

func TestSQLiteStartupRejectsUnsupportedSessionAggregateEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wingman.db")
	data, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.db.Exec(`INSERT INTO aggregate_events (id, aggregate_type, aggregate_id, version, event_type, schema_version, payload_json, created_at) VALUES ('evt_unsupported', 'session', 'ses_unsupported', 1, 'session.future', 99, '{}', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := data.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = NewSQLiteStore(path)
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("NewSQLiteStore error = %v, want unsupported aggregate event", err)
	}
}

func TestSQLiteRebuildAllSessionProjectionsRollsBackOnInvalidHistory(t *testing.T) {
	data := newTestSQLiteStore(t)
	ctx := context.Background()
	for _, id := range []string{"ses_rebuild_valid", "ses_rebuild_invalid"} {
		if err := data.CreateSession(&Session{ID: id, Title: id}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := data.db.Exec(`UPDATE aggregate_events SET schema_version = 99 WHERE aggregate_type = 'session' AND aggregate_id = 'ses_rebuild_invalid'`); err != nil {
		t.Fatal(err)
	}
	if _, err := data.db.Exec(`UPDATE sessions SET title = 'disturbed' WHERE id = 'ses_rebuild_valid'`); err != nil {
		t.Fatal(err)
	}
	if err := data.RebuildAllSessionProjections(ctx); err == nil {
		t.Fatal("RebuildAllSessionProjections succeeded with invalid history")
	}
	valid, err := data.GetSession("ses_rebuild_valid")
	if err != nil || valid.Title != "disturbed" {
		t.Fatalf("valid session = %#v, %v; want untouched disturbed projection", valid, err)
	}
}

func TestSQLiteMessageSnapshotsProjectFromAggregateHistory(t *testing.T) {
	data := newTestSQLiteStore(t)
	ctx := context.Background()
	if err := data.CreateSession(&Session{ID: "ses_message_events"}); err != nil {
		t.Fatal(err)
	}
	message := StoredMessage{ID: "msg_events", SessionID: "ses_message_events", Idx: 1, Role: "assistant", Revision: 1, State: "in_progress", Parts: []StoredPart{{ID: "prt_events", MessageID: "msg_events", Sequence: 0, Kind: "text", PayloadJSON: []byte(`{"text":"first"}`)}}}
	if err := data.SaveMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	message.Revision, message.State, message.Parts[0].PayloadJSON = 2, "completed", []byte(`{"text":"second"}`)
	if err := data.SaveMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	events, err := data.ListAggregateEvents(ctx, AggregateRef{Type: AggregateSession, ID: "ses_message_events"}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[1].Type != EventSessionMessageSaved || events[2].Type != EventSessionMessageSaved {
		t.Fatalf("aggregate events = %#v", events)
	}
	projected, err := ProjectSessionMessages(events)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := data.ListMessages(ctx, "ses_message_events")
	if err != nil || !reflect.DeepEqual(projected, stored) {
		t.Fatalf("projected = %#v, stored = %#v, error = %v", projected, stored, err)
	}
}

func TestSQLiteToolUseSnapshotsProjectFromAggregateHistory(t *testing.T) {
	data := newTestSQLiteStore(t)
	ctx := context.Background()
	if err := data.CreateSession(&Session{ID: "ses_tool_events"}); err != nil {
		t.Fatal(err)
	}
	run, err := data.AdmitSessionRun(ctx, SessionRun{ID: "run_tool_events", SessionID: "ses_tool_events"})
	if err != nil {
		t.Fatal(err)
	}
	use := ToolUse{ID: "tlu_events", SessionID: "ses_tool_events", RunID: run.Run.ID, Step: 1, Ordinal: 0, CallID: "call_events", Name: "shell", Status: ToolUseStatusProposed, InputJSON: []byte(`{"command":"pwd"}`)}
	for _, status := range []string{ToolUseStatusProposed, ToolUseStatusAuthorized, ToolUseStatusStarted, ToolUseStatusCompleted} {
		use.Status = status
		if status == ToolUseStatusAuthorized {
			use.InputJSON = []byte(`not json, intentionally opaque`)
		}
		if status == ToolUseStatusCompleted {
			use.Output = "ok"
			use.StructuredJSON = []byte(`also opaque`)
			use.MetadataJSON = []byte(`metadata bytes`)
		}
		if err := data.SaveToolUse(ctx, use); err != nil {
			t.Fatal(err)
		}
	}
	events, err := data.ListAggregateEvents(ctx, AggregateRef{Type: AggregateSession, ID: use.SessionID}, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := ProjectSessionToolUses(events)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := data.ListToolUses(ctx, use.SessionID)
	if err != nil || !reflect.DeepEqual(projected, stored) {
		t.Fatalf("projected = %#v, stored = %#v, error = %v", projected, stored, err)
	}
}

func TestSQLiteToolUseSnapshotRollsBackWithAggregateEvent(t *testing.T) {
	data := newTestSQLiteStore(t)
	ctx := context.Background()
	if err := data.CreateSession(&Session{ID: "ses_tool_rollback"}); err != nil {
		t.Fatal(err)
	}
	if _, err := data.db.Exec(`CREATE TRIGGER fail_tool_event BEFORE INSERT ON aggregate_events WHEN NEW.event_type = 'session.tool_use.saved' BEGIN SELECT RAISE(ABORT, 'reject'); END`); err != nil {
		t.Fatal(err)
	}
	err := data.SaveToolUse(ctx, ToolUse{ID: "tlu_rollback", SessionID: "ses_tool_rollback", Name: "read", Status: ToolUseStatusProposed})
	if err == nil {
		t.Fatal("SaveToolUse succeeded")
	}
	uses, err := data.ListToolUses(ctx, "ses_tool_rollback")
	if err != nil || len(uses) != 0 {
		t.Fatalf("tool uses = %#v, error = %v", uses, err)
	}
	session, err := data.GetSession("ses_tool_rollback")
	if err != nil || session.AggregateVersion != 1 {
		t.Fatalf("session = %#v, error = %v", session, err)
	}
}

func TestSQLiteMessageSnapshotRollsBackWithAggregateEvent(t *testing.T) {
	data := newTestSQLiteStore(t)
	ctx := context.Background()
	if err := data.CreateSession(&Session{ID: "ses_message_rollback"}); err != nil {
		t.Fatal(err)
	}
	if _, err := data.db.Exec(`CREATE TRIGGER fail_message_event BEFORE INSERT ON aggregate_events WHEN NEW.event_type = 'session.message.saved' BEGIN SELECT RAISE(ABORT, 'reject'); END`); err != nil {
		t.Fatal(err)
	}
	err := data.SaveMessage(ctx, StoredMessage{ID: "msg_rollback", SessionID: "ses_message_rollback", Idx: 1, Role: "user", Parts: []StoredPart{{ID: "prt_rollback", MessageID: "msg_rollback", Sequence: 0, Kind: "text", PayloadJSON: []byte(`{"text":"nope"}`)}}})
	if err == nil {
		t.Fatal("SaveMessage succeeded")
	}
	messages, err := data.ListMessages(ctx, "ses_message_rollback")
	if err != nil || len(messages) != 0 {
		t.Fatalf("messages = %#v, error = %v", messages, err)
	}
}

func TestSQLitePermissionRequestRollsBackWithAggregateEvent(t *testing.T) {
	data := newTestSQLiteStore(t)
	ctx := context.Background()
	if err := data.CreateSession(&Session{ID: "ses_permission_rollback"}); err != nil {
		t.Fatal(err)
	}
	if _, err := data.db.Exec(`CREATE TRIGGER fail_permission_event BEFORE INSERT ON aggregate_events WHEN NEW.event_type = 'session.permission.requested' BEGIN SELECT RAISE(ABORT, 'reject'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreatePermissionRequest(ctx, PermissionRequest{ID: "prq_rollback", SessionID: "ses_permission_rollback", Action: "shell.exec", Resources: []string{"pwd"}}); err == nil {
		t.Fatal("CreatePermissionRequest succeeded")
	}
	requests, err := data.ListPermissionRequests(ctx, "ses_permission_rollback")
	if err != nil || len(requests) != 0 {
		t.Fatalf("requests = %#v, error = %v", requests, err)
	}
	if watermark, err := data.SessionEventWatermark(ctx, "ses_permission_rollback"); err != nil || watermark != 0 {
		t.Fatalf("watermark = %d, error = %v", watermark, err)
	}
	session, err := data.GetSession("ses_permission_rollback")
	if err != nil || session.AggregateVersion != 1 {
		t.Fatalf("session = %#v, error = %v", session, err)
	}
}

func TestSQLiteSessionMetadataNoOpAndRollback(t *testing.T) {
	data := newTestSQLiteStore(t)
	session := &Session{ID: "ses_noop", Title: "Same"}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	unchanged, err := data.RenameSession(context.Background(), session.ID, "Same", 1)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.AggregateVersion != 1 {
		t.Fatalf("no-op version = %d, want 1", unchanged.AggregateVersion)
	}
	if _, err := data.MoveSession(context.Background(), session.ID, "/missing", "wsp_missing", 1); err == nil {
		t.Fatal("MoveSession succeeded with missing workspace")
	}
	events, err := data.ListAggregateEvents(context.Background(), AggregateRef{Type: AggregateSession, ID: session.ID}, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}

func TestSQLiteSessionMetadataSerializesConcurrentWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wingman.db")
	first, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	session := &Session{ID: "ses_metadata_concurrent", Title: "Before"}
	if err := first.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i, data := range []*SQLiteStore{first, second} {
		wg.Add(1)
		go func(data *SQLiteStore, title string) {
			defer wg.Done()
			<-start
			_, err := data.RenameSession(context.Background(), session.ID, title, 1)
			errs <- err
		}(data, []string{"First", "Second"}[i])
	}
	close(start)
	wg.Wait()
	close(errs)
	var successes, conflicts int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrAggregateVersionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes = %d, conflicts = %d; want 1 each", successes, conflicts)
	}
}

func TestSQLitePurgeSessionRemovesAllDurableState(t *testing.T) {
	data := newTestSQLiteStore(t)
	ctx := context.Background()
	session := &Session{ID: "ses_purge", Title: "Purge me"}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := data.SaveMessage(ctx, StoredMessage{
		ID: "msg_purge", SessionID: session.ID, Role: "user", Revision: 1, State: "completed", CreatedAt: now, UpdatedAt: now,
		Parts: []StoredPart{{ID: "prt_purge", MessageID: "msg_purge", Kind: "text", PayloadJSON: []byte(`{"text":"purge"}`), CreatedAt: now, UpdatedAt: now}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := data.UpsertModelCall(ctx, ModelCall{ID: "mcl_purge", SessionID: session.ID, Step: 1, Status: ModelCallStatusCompleted}); err != nil {
		t.Fatal(err)
	}
	if _, err := data.AdmitSessionRun(ctx, SessionRun{ID: "run_purge", SessionID: session.ID, Message: "purge", Agent: Agent{ID: "agt_test"}}); err != nil {
		t.Fatal(err)
	}
	if err := data.SaveToolUse(ctx, ToolUse{ID: "tlu_purge", SessionID: session.ID, RunID: "run_purge", Step: 1, Ordinal: 0, Name: "read", Status: ToolUseStatusProposed}); err != nil {
		t.Fatal(err)
	}
	if _, err := data.AppendSessionEvent(ctx, SessionEvent{ID: "evt_public_purge", SessionID: session.ID, Type: "session.run.queued"}); err != nil {
		t.Fatal(err)
	}

	if err := data.PurgeSession(ctx, session.ID, 1); !errors.Is(err, ErrAggregateVersionConflict) {
		t.Fatalf("stale purge error = %v, want version conflict", err)
	}
	if err := data.PurgeSession(ctx, session.ID, 5); err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		name  string
		query string
		arg   string
	}{
		{"sessions", `SELECT COUNT(*) FROM sessions WHERE id = ?`, session.ID},
		{"aggregate events", `SELECT COUNT(*) FROM aggregate_events WHERE aggregate_type = 'session' AND aggregate_id = ?`, session.ID},
		{"runs", `SELECT COUNT(*) FROM session_runs WHERE session_id = ?`, session.ID},
		{"public events", `SELECT COUNT(*) FROM session_events WHERE session_id = ?`, session.ID},
		{"messages", `SELECT COUNT(*) FROM messages WHERE session_id = ?`, session.ID},
		{"parts", `SELECT COUNT(*) FROM parts WHERE id = ?`, "prt_purge"},
		{"model calls", `SELECT COUNT(*) FROM model_calls WHERE session_id = ?`, session.ID},
		{"tool uses", `SELECT COUNT(*) FROM tool_uses WHERE session_id = ?`, session.ID},
	}
	for _, check := range checks {
		var count int
		if err := data.db.QueryRow(check.query, check.arg).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", check.name, err)
		}
		if count != 0 {
			t.Fatalf("%s count = %d, want 0", check.name, count)
		}
	}
}

func TestSQLitePurgeSessionRollsBackAggregateDeletion(t *testing.T) {
	data := newTestSQLiteStore(t)
	session := &Session{ID: "ses_purge_rollback"}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	if _, err := data.db.Exec(`
		CREATE TRIGGER fail_session_purge
		BEFORE DELETE ON sessions
		WHEN OLD.id = 'ses_purge_rollback'
		BEGIN
			SELECT RAISE(ABORT, 'forced purge failure');
		END
	`); err != nil {
		t.Fatal(err)
	}
	if err := data.PurgeSession(context.Background(), session.ID, 1); err == nil {
		t.Fatal("PurgeSession succeeded, want forced projection failure")
	}
	if _, err := data.GetSession(session.ID); err != nil {
		t.Fatalf("session was not restored: %v", err)
	}
	events, err := data.ListAggregateEvents(context.Background(), AggregateRef{Type: AggregateSession, ID: session.ID}, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("aggregate events = %d, want 1 after rollback", len(events))
	}
}

func TestSQLitePurgeAndRenameSerializeAcrossStoreHandles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wingman.db")
	first, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	session := &Session{ID: "ses_purge_concurrent", Title: "Before"}
	if err := first.CreateSession(session); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	renameErr := make(chan error, 1)
	purgeErr := make(chan error, 1)
	go func() {
		<-start
		_, err := first.RenameSession(context.Background(), session.ID, "After", 1)
		renameErr <- err
	}()
	go func() {
		<-start
		purgeErr <- second.PurgeSession(context.Background(), session.ID, 1)
	}()
	close(start)
	rErr, pErr := <-renameErr, <-purgeErr
	switch {
	case rErr == nil && errors.Is(pErr, ErrAggregateVersionConflict):
		stored, err := first.GetSession(session.ID)
		if err != nil || stored.AggregateVersion != 2 {
			t.Fatalf("rename winner projection = %#v, error = %v", stored, err)
		}
	case pErr == nil && errors.Is(rErr, ErrSessionNotFound):
		if _, err := first.GetSession(session.ID); err == nil {
			t.Fatal("purge won but session still exists")
		}
	default:
		t.Fatalf("rename error = %v, purge error = %v", rErr, pErr)
	}
}

func TestProjectSessionIsDeterministic(t *testing.T) {
	session := Session{
		ID:        "ses_replay",
		Title:     "Replay me",
		CreatedAt: "2026-07-30T12:00:00Z",
		UpdatedAt: "2026-07-30T12:00:00Z",
	}
	event, err := NewSessionCreatedEvent(session)
	if err != nil {
		t.Fatal(err)
	}
	event.Version = 1
	first, err := ProjectSession([]AggregateEvent{event})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ProjectSession([]AggregateEvent{event})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("first = %#v, second = %#v", first, second)
	}
}

func TestProjectSessionAdmissionOnlyAdvancesVersion(t *testing.T) {
	session := Session{ID: "ses_admission_projection", Title: "unchanged", UpdatedAt: "2026-07-30T12:00:00Z", CreatedAt: "2026-07-30T12:00:00Z"}
	created, err := NewSessionCreatedEvent(session)
	if err != nil {
		t.Fatal(err)
	}
	created.Version = 1
	run := SessionRun{ID: "run_projection", SessionID: session.ID, AdmittedVersion: 2, Status: SessionRunStatusQueued, CreatedAt: time.Date(2026, 7, 30, 12, 1, 0, 0, time.UTC)}
	admitted, err := NewSessionRunAdmittedEvent(run)
	if err != nil {
		t.Fatal(err)
	}
	admitted.Version = 2
	projected, err := ProjectSession([]AggregateEvent{created, admitted})
	if err != nil {
		t.Fatal(err)
	}
	if projected.AggregateVersion != 2 || projected.UpdatedAt != session.UpdatedAt || projected.Title != session.Title {
		t.Fatalf("projected = %#v", projected)
	}
}

func TestSessionRunAggregateRoundTripPreservesActionInput(t *testing.T) {
	run := SessionRun{
		ID: "run_action_projection", SessionID: "ses_action_projection", Kind: SessionRunKindAction,
		Action: "example.run", InputJSON: []byte(`{"value":1}`), RequestHash: "hash",
		AdmittedVersion: 1, Status: SessionRunStatusQueued, CreatedAt: time.Now().UTC(),
	}
	data, err := marshalSessionRun(run)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := unmarshalSessionRun(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projected.InputJSON, run.InputJSON) || projected.RequestHash != run.RequestHash {
		t.Fatalf("projected = %#v, want input %s and request hash %q", projected, run.InputJSON, run.RequestHash)
	}
}

func TestProjectSessionRejectsInvalidStream(t *testing.T) {
	_, err := ProjectSession([]AggregateEvent{{
		Aggregate:     AggregateRef{Type: AggregateSession, ID: "ses_invalid"},
		Version:       2,
		Type:          EventSessionCreated,
		SchemaVersion: 1,
		Data:          []byte(`{"id":"ses_invalid"}`),
	}})
	if err == nil {
		t.Fatal("ProjectSession succeeded with a gapped stream")
	}
}

func TestProjectSessionRejectsMixedAggregates(t *testing.T) {
	created, err := NewSessionCreatedEvent(Session{
		ID:        "ses_first",
		CreatedAt: "2026-07-30T12:00:00Z",
		UpdatedAt: "2026-07-30T12:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	created.Version = 1
	renamed, err := NewSessionRenamedEvent("ses_second", "Wrong stream", "2026-07-30T12:01:00Z")
	if err != nil {
		t.Fatal(err)
	}
	renamed.Version = 2
	if _, err := ProjectSession([]AggregateEvent{created, renamed}); err == nil {
		t.Fatal("ProjectSession succeeded with mixed aggregate IDs")
	}
}

func newTestSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	data, err := NewSQLiteStore(filepath.Join(t.TempDir(), "wingman.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	return data
}
