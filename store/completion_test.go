package store_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestCompletionTransitionsAndReplay(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			var data store.Store = memory.NewStore()
			if backend == "sqlite" {
				db, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "completion.db"))
				if err != nil {
					t.Fatal(err)
				}
				data = db
			}
			defer data.Close()
			ctx := t.Context()
			if err := data.CreateSession(&store.Session{ID: "ses_completion"}); err != nil {
				t.Fatal(err)
			}
			admitted, err := data.AdmitSessionRun(ctx, store.SessionRun{SessionID: "ses_completion", Completion: store.CompletionOptions{Remote: true}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := data.ClaimNextSessionRun(ctx, "ses_completion"); err != nil {
				t.Fatal(err)
			}
			update := store.CompletionUpdate{SessionID: "ses_completion", RunID: admitted.Run.ID, Completion: store.Completion{CandidateID: "msg_candidate", Step: 1, Status: "validating"}}
			if _, err := data.UpdateSessionRunCompletion(ctx, update); err != nil {
				t.Fatal(err)
			}
			update.ExpectedStatus, update.Completion.Status = "validating", "waiting"
			if _, err := data.UpdateSessionRunCompletion(ctx, update); err != nil {
				t.Fatal(err)
			}
			next, err := data.AdmitSessionRun(ctx, store.SessionRun{SessionID: "ses_completion", Message: "later"})
			if err != nil {
				t.Fatal(err)
			}
			claimed, err := data.ClaimNextSessionRun(ctx, "ses_completion")
			if err != nil || claimed.Run.ID != "" {
				t.Fatalf("waiting run did not block queue: %#v %v", claimed, err)
			}
			update.ExpectedStatus, update.Completion.Status, update.Completion.Feedback = "waiting", "continue", "fix tests"
			var changes atomic.Int32
			var wg sync.WaitGroup
			for range 8 {
				wg.Go(func() {
					result, err := data.UpdateSessionRunCompletion(ctx, update)
					if err != nil {
						t.Error(err)
					}
					if result.Changed {
						changes.Add(1)
					}
				})
			}
			wg.Wait()
			if changes.Load() != 1 {
				t.Fatalf("commits=%d", changes.Load())
			}
			conflict := update
			conflict.Completion.Feedback = "different"
			if _, err := data.UpdateSessionRunCompletion(ctx, conflict); !errors.Is(err, store.ErrSessionRunTransitionConflict) {
				t.Fatalf("conflicting reply=%v", err)
			}
			claimed, err = data.ClaimNextSessionRun(ctx, "ses_completion")
			if err != nil || claimed.Run.ID != admitted.Run.ID || claimed.Run.RecoveryAttempts != 0 {
				t.Fatalf("resumed wrong run: %#v %v", claimed, err)
			}
			update.ExpectedStatus = ""
			update.Completion = store.Completion{CandidateID: "msg_second", Step: 2, Status: "validating"}
			if _, err := data.UpdateSessionRunCompletion(ctx, update); err != nil {
				t.Fatal(err)
			}
			update.ExpectedStatus, update.Completion.Status = "validating", "waiting"
			if _, err := data.UpdateSessionRunCompletion(ctx, update); err != nil {
				t.Fatal(err)
			}
			if _, err := data.SettleSessionRun(ctx, store.SessionRunSettlement{ID: admitted.Run.ID, ExpectedStatus: store.SessionRunStatusAwaitingCompletion, Status: store.SessionRunStatusAborted}); err != nil {
				t.Fatal(err)
			}
			update.ExpectedStatus, update.Completion.Status = "waiting", "accept"
			if _, err := data.UpdateSessionRunCompletion(ctx, update); !errors.Is(err, store.ErrSessionRunTransitionConflict) {
				t.Fatalf("revived canceled run: %v", err)
			}
			claimed, err = data.ClaimNextSessionRun(ctx, "ses_completion")
			if err != nil || claimed.Run.ID != next.Run.ID {
				t.Fatalf("queue stalled: %#v %v", claimed, err)
			}
			events, err := data.(store.AggregateEventReader).ListAggregateEvents(ctx, store.AggregateRef{Type: store.AggregateSession, ID: "ses_completion"}, 0, 1000)
			if err != nil {
				t.Fatal(err)
			}
			projected, err := store.ProjectSessionAggregate(events)
			if err != nil {
				t.Fatal(err)
			}
			runs, err := data.ListSessionRuns(ctx, "ses_completion")
			if err != nil || !reflect.DeepEqual(projected.Runs, runs) {
				t.Fatalf("projection differs: %#v\n%#v\n%v", projected.Runs, runs, err)
			}
			if runs[0].Completions[0].FeedbackID == "" {
				t.Fatal("missing feedback identity")
			}
			if db, ok := data.(*store.SQLiteStore); ok {
				if err := db.RebuildSessionProjections(ctx, "ses_completion"); err != nil {
					t.Fatal(err)
				}
				rebuilt, err := data.ListSessionRuns(ctx, "ses_completion")
				if err != nil || !reflect.DeepEqual(rebuilt, runs) {
					t.Fatalf("rebuilt=%#v err=%v", rebuilt, err)
				}
			}
		})
	}
}
