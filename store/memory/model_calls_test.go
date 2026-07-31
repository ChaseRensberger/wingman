package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/store"
)

func TestModelCallAttemptsMatchSQLiteContract(t *testing.T) {
	data := NewStore()
	ctx := context.Background()
	session := &store.Session{ID: "ses_model_calls"}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	first, err := data.AdmitSessionRun(ctx, store.SessionRun{ID: "run_first", SessionID: session.ID, Message: "first"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := data.AdmitSessionRun(ctx, store.SessionRun{ID: "run_second", SessionID: session.ID, Message: "second"})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	for _, call := range []store.ModelCall{
		{ID: "mcl_first", SessionID: session.ID, RunID: first.Run.ID, Step: 1, Status: store.ModelCallStatusStarted, StartedAt: started},
		{ID: "mcl_second", SessionID: session.ID, RunID: second.Run.ID, Step: 1, Status: store.ModelCallStatusStarted, StartedAt: started.Add(time.Minute)},
	} {
		if err := data.UpsertModelCall(ctx, call); err != nil {
			t.Fatal(err)
		}
	}
	if err := data.UpsertModelCall(ctx, store.ModelCall{ID: "mcl_first", SessionID: session.ID, RunID: first.Run.ID, Step: 1, Status: store.ModelCallStatusCompleted, ProviderRequestID: "request-1", StartedAt: started.Add(time.Hour), CompletedAt: started.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := data.UpsertModelCall(ctx, store.ModelCall{ID: "mcl_conflict", SessionID: session.ID, RunID: first.Run.ID, Step: 1, Status: store.ModelCallStatusFailed}); !errors.Is(err, store.ErrModelCallAttemptConflict) {
		t.Fatalf("conflict error = %v", err)
	}
	calls, err := data.ListModelCalls(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0].ID != "mcl_first" || calls[1].ID != "mcl_second" {
		t.Fatalf("calls = %#v", calls)
	}
	if !calls[0].StartedAt.Equal(started) || calls[0].Status != store.ModelCallStatusCompleted || calls[0].ProviderRequestID != "request-1" {
		t.Fatalf("updated call = %#v", calls[0])
	}
}

func TestToolUseLifecycleMatchesSQLiteContract(t *testing.T) {
	data := NewStore()
	ctx := context.Background()
	session := &store.Session{ID: "ses_tools"}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	run, err := data.AdmitSessionRun(ctx, store.SessionRun{ID: "run_tools", SessionID: session.ID})
	if err != nil {
		t.Fatal(err)
	}
	use := store.ToolUse{ID: "tlu_test", SessionID: session.ID, RunID: run.Run.ID, Step: 1, Ordinal: 0, CallID: "call_1", Name: "read", Status: store.ToolUseStatusProposed, InputJSON: []byte(`{"path":"a"}`)}
	if err := data.SaveToolUse(ctx, use); err != nil {
		t.Fatal(err)
	}
	use.Status = store.ToolUseStatusAuthorized
	use.InputJSON[9] = 'b'
	if err := data.SaveToolUse(ctx, use); err != nil {
		t.Fatal(err)
	}
	if err := data.InterruptActiveToolUses(ctx); err != nil {
		t.Fatal(err)
	}
	uses, err := data.ListToolUses(ctx, session.ID)
	if err != nil || len(uses) != 1 || uses[0].Status != store.ToolUseStatusInterrupted || uses[0].ErrorType != "process_interrupted" {
		t.Fatalf("uses = %#v, error = %v", uses, err)
	}
	uses[0].InputJSON[0] = '['
	again, err := data.ListToolUses(ctx, session.ID)
	if err != nil || again[0].InputJSON[0] != '{' {
		t.Fatalf("tool use JSON was not deep copied: %#v, %v", again, err)
	}
}
