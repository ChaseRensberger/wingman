package memory

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/store"
)

func TestCreateSessionCommitsEventAndProjection(t *testing.T) {
	data := NewStore()
	session := &store.Session{ID: "ses_memory", Title: "Memory"}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	events, err := data.ListAggregateEvents(context.Background(), store.AggregateRef{Type: store.AggregateSession, ID: session.ID}, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := store.ProjectSession(events)
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

func TestCreateSessionRejectsDuplicateAggregate(t *testing.T) {
	data := NewStore()
	if err := data.CreateSession(&store.Session{ID: "ses_duplicate"}); err != nil {
		t.Fatal(err)
	}
	err := data.CreateSession(&store.Session{ID: "ses_duplicate"})
	if !errors.Is(err, store.ErrAggregateVersionConflict) {
		t.Fatalf("error = %v, want aggregate version conflict", err)
	}
	events, listErr := data.ListAggregateEvents(context.Background(), store.AggregateRef{Type: store.AggregateSession, ID: "ses_duplicate"}, 0, 100)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}

func TestSessionMetadataEventsUpdateProjectionAndReplay(t *testing.T) {
	data := NewStore()
	session := &store.Session{ID: "ses_update", Title: "Before", WorkDir: "/before"}
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
	if moved.AggregateVersion != 3 || moved.Title != "After" || moved.WorkDir != "/after" {
		t.Fatalf("moved session = %#v", moved)
	}
	events, err := data.ListAggregateEvents(context.Background(), store.AggregateRef{Type: store.AggregateSession, ID: session.ID}, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := store.ProjectSession(events)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projected, moved) {
		t.Fatalf("replayed projection = %#v, stored = %#v", projected, moved)
	}
}

func TestSessionMetadataNoOpAndConflict(t *testing.T) {
	data := NewStore()
	session := &store.Session{ID: "ses_conflict", Title: "Before"}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	unchanged, err := data.RenameSession(context.Background(), session.ID, "Before", 1)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.AggregateVersion != 1 {
		t.Fatalf("no-op version = %d, want 1", unchanged.AggregateVersion)
	}
	if _, err := data.RenameSession(context.Background(), session.ID, "After", 1); err != nil {
		t.Fatal(err)
	}
	_, err = data.MoveSession(context.Background(), session.ID, "/stale", "", 1)
	if !errors.Is(err, store.ErrAggregateVersionConflict) {
		t.Fatalf("error = %v, want aggregate version conflict", err)
	}
	events, listErr := data.ListAggregateEvents(context.Background(), store.AggregateRef{Type: store.AggregateSession, ID: session.ID}, 0, 100)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
}

func TestPurgeSessionRemovesAllState(t *testing.T) {
	data := NewStore()
	ctx := context.Background()
	session := &store.Session{ID: "ses_purge"}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := data.SaveMessage(ctx, store.StoredMessage{
		ID: "msg_purge", SessionID: session.ID, Role: "user", Revision: 1, State: "completed", CreatedAt: now, UpdatedAt: now,
		Parts: []store.StoredPart{{ID: "prt_purge", MessageID: "msg_purge", Kind: "text", PayloadJSON: []byte(`{}`), CreatedAt: now, UpdatedAt: now}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := data.UpsertModelCall(ctx, store.ModelCall{ID: "mcl_purge", SessionID: session.ID, Step: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := data.AdmitSessionRun(ctx, store.SessionRun{ID: "run_purge", SessionID: session.ID}); err != nil {
		t.Fatal(err)
	}
	if err := data.SaveToolUse(ctx, store.ToolUse{ID: "tlu_purge", SessionID: session.ID, RunID: "run_purge", Step: 1, Ordinal: 0, Name: "read", Status: store.ToolUseStatusProposed}); err != nil {
		t.Fatal(err)
	}
	if _, err := data.AppendSessionEvent(ctx, store.SessionEvent{ID: "evt_purge", SessionID: session.ID}); err != nil {
		t.Fatal(err)
	}
	if err := data.PurgeSession(ctx, session.ID, 1); !errors.Is(err, store.ErrAggregateVersionConflict) {
		t.Fatalf("stale purge error = %v, want version conflict", err)
	}
	if err := data.PurgeSession(ctx, session.ID, 2); err != nil {
		t.Fatal(err)
	}
	if _, ok := data.sessions[session.ID]; ok {
		t.Fatal("session projection remains after purge")
	}
	if _, ok := data.aggregates[store.AggregateRef{Type: store.AggregateSession, ID: session.ID}]; ok {
		t.Fatal("aggregate history remains after purge")
	}
	if len(data.messages) != 0 || len(data.parts) != 0 || len(data.modelCalls) != 0 || len(data.toolUses) != 0 || len(data.runs) != 0 || len(data.events) != 0 {
		t.Fatalf("state remains after purge: messages=%d parts=%d calls=%d tool_uses=%d runs=%d events=%d", len(data.messages), len(data.parts), len(data.modelCalls), len(data.toolUses), len(data.runs), len(data.events))
	}
}

func TestAdmitSessionRunMatchesSQLiteContract(t *testing.T) {
	data := NewStore()
	if err := data.CreateSession(&store.Session{ID: "ses_admit_memory", WorkDir: "/memory"}); err != nil {
		t.Fatal(err)
	}
	first, err := data.AdmitSessionRun(context.Background(), store.SessionRun{SessionID: "ses_admit_memory", RequestID: "request", Message: "hello", Agent: store.Agent{ID: "agt"}})
	if err != nil {
		t.Fatal(err)
	}
	retry, err := data.AdmitSessionRun(context.Background(), store.SessionRun{SessionID: "ses_admit_memory", RequestID: "request", Message: "hello", Agent: store.Agent{ID: "agt"}})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created || retry.Created || retry.Run.ID != first.Run.ID || first.SessionVersion != 2 || first.Run.WorkDir != "/memory" {
		t.Fatalf("first=%#v retry=%#v", first, retry)
	}
	if _, err := data.AdmitSessionRun(context.Background(), store.SessionRun{SessionID: "ses_admit_memory", RequestID: "request", Message: "different", Agent: store.Agent{ID: "agt"}}); !errors.Is(err, store.ErrSessionRunAdmissionConflict) {
		t.Fatalf("error = %v", err)
	}
	events, err := data.ListAggregateEvents(context.Background(), store.AggregateRef{Type: store.AggregateSession, ID: "ses_admit_memory"}, 0, 10)
	if err != nil || len(events) != 2 || events[1].Type != store.EventSessionRunAdmitted {
		t.Fatalf("events=%#v error=%v", events, err)
	}
	queued, err := data.ListSessionEvents(context.Background(), "ses_admit_memory", 0, 10)
	if err != nil || len(queued) != 1 || queued[0].Type != "session.run.queued" {
		t.Fatalf("queued=%#v error=%v", queued, err)
	}
}
