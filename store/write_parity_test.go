package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestMessageAndModelCallWriteParity(t *testing.T) {
	for _, backend := range []struct {
		name string
		open func(*testing.T) store.Store
	}{
		{"memory", func(*testing.T) store.Store { return memory.NewStore() }},
		{"sqlite", func(t *testing.T) store.Store {
			data, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "wingman.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = data.Close() })
			return data
		}},
	} {
		t.Run(backend.name, func(t *testing.T) {
			ctx, data := context.Background(), backend.open(t)
			if err := data.CreateSession(&store.Session{ID: "ses_parity"}); err != nil {
				t.Fatal(err)
			}
			run, err := data.AdmitSessionRun(ctx, store.SessionRun{ID: "run_parity", SessionID: "ses_parity", Message: "hello"})
			if err != nil {
				t.Fatal(err)
			}
			claimed, err := data.ClaimNextSessionRun(ctx, "ses_parity")
			if err != nil || !claimed.Changed || claimed.Run.ID != run.Run.ID {
				t.Fatalf("claim = %#v, %v", claimed, err)
			}
			message := store.StoredMessage{ID: "msg_parity", SessionID: "ses_parity", RunID: run.Run.ID, Role: "assistant", State: "in_progress", Revision: 1, Parts: []store.StoredPart{{ID: "prt_parity", MessageID: "msg_parity", Kind: "text", PayloadJSON: []byte(`{"text":"a"}`)}}}
			if err := data.SaveMessage(ctx, message); err != nil {
				t.Fatal(err)
			}
			message.Revision++
			message.Parts[0].PayloadJSON = []byte(`{"text":"ab"}`)
			if err := data.SaveMessage(ctx, message); err != nil {
				t.Fatal(err)
			}
			if err := data.SaveMessage(ctx, message); err != nil {
				t.Fatalf("same revision should be idempotent: %v", err)
			}
			if err := data.SaveMessage(ctx, store.StoredMessage{ID: message.ID, SessionID: message.SessionID, RunID: message.RunID, Role: message.Role, Revision: 1}); !errors.Is(err, store.ErrMessageRevisionStale) {
				t.Fatalf("stale revision: %v", err)
			}
			call := store.ModelCall{ID: "mcl_parity", SessionID: "ses_parity", RunID: run.Run.ID, Step: 1, Status: store.ModelCallStatusStarted, AssistantMessageID: message.ID}
			if err := data.UpsertModelCall(ctx, call); err != nil {
				t.Fatal(err)
			}
			call.Status = store.ModelCallStatusFailed
			if err := data.UpsertModelCall(ctx, call); err != nil {
				t.Fatal(err)
			}
			if err := data.UpsertModelCall(ctx, store.ModelCall{ID: "mcl_duplicate", SessionID: "ses_parity", RunID: run.Run.ID, Step: 1, Attempt: 1, Status: store.ModelCallStatusStarted}); !errors.Is(err, store.ErrModelCallAttemptConflict) {
				t.Fatalf("duplicate attempt: %v", err)
			}
			settled, err := data.SettleSessionRun(ctx, store.SessionRunSettlement{ID: run.Run.ID, ExpectedStatus: store.SessionRunStatusRunning, Status: store.SessionRunStatusCompleted})
			if err != nil || !settled.Changed || settled.Run.Status != store.SessionRunStatusCompleted {
				t.Fatalf("settle = %#v, %v", settled, err)
			}
			events, err := data.(store.AggregateEventReader).ListAggregateEvents(ctx, store.AggregateRef{Type: store.AggregateSession, ID: "ses_parity"}, 0, 100)
			if err != nil {
				t.Fatal(err)
			}
			projection, err := store.ProjectSessionAggregate(events)
			if err != nil {
				t.Fatal(err)
			}
			got, err := data.GetSession("ses_parity")
			if err != nil || !reflect.DeepEqual(got, projection.Session) {
				t.Fatalf("session projection = %#v, %v; replay = %#v", got, err, projection.Session)
			}
			messages, err := data.ListMessages(ctx, "ses_parity")
			if err != nil || len(messages) != 1 || messages[0].Revision != 2 || string(messages[0].Parts[0].PayloadJSON) != `{"text":"ab"}` {
				t.Fatalf("messages = %#v, %v", messages, err)
			}
			calls, err := data.ListModelCalls(ctx, "ses_parity")
			if err != nil || len(calls) != 1 || calls[0].Status != store.ModelCallStatusFailed {
				t.Fatalf("model calls = %#v, %v", calls, err)
			}
		})
	}
}
