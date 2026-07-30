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
