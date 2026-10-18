package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/chaserensberger/wingman/question"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestQuestionsDurableTransitions(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			var data store.Store = memory.NewStore()
			if backend == "sqlite" {
				db, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "questions.db"))
				if err != nil {
					t.Fatal(err)
				}
				data = db
			}
			defer data.Close()
			ctx := t.Context()
			const sid = "ses_question"
			if err := data.CreateSession(&store.Session{ID: sid}); err != nil {
				t.Fatal(err)
			}
			admission, err := data.AdmitSessionRun(ctx, store.SessionRun{SessionID: sid})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := data.ClaimNextSessionRun(ctx, sid); err != nil {
				t.Fatal(err)
			}
			create := func(id, runID string) question.Question {
				t.Helper()
				result, err := data.CreateQuestion(ctx, question.Question{ID: id, SessionID: sid, RunID: runID, Questions: []question.Prompt{{Header: "Choice", Question: "Which?", Options: []question.Choice{{Label: "One", Description: "First"}}}}})
				if err != nil {
					t.Fatal(err)
				}
				return result.Question
			}
			q := create("que_first", admission.Run.ID)
			second := create("que_second", admission.Run.ID)
			if _, err := data.SuspendSessionRunForQuestions(ctx, sid, admission.Run.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := data.AdmitSessionRun(ctx, store.SessionRun{SessionID: sid, Message: "later"}); err != nil {
				t.Fatal(err)
			}
			if result, err := data.ClaimNextSessionRun(ctx, sid); err != nil || result.Run.ID != "" {
				t.Fatalf("waiting lost queue ownership: %#v %v", result, err)
			}
			if _, err := data.ResolveQuestion(ctx, "ses_other", q.ID, question.Reply{Status: "dismissed"}); !errors.Is(err, question.ErrNotFound) {
				t.Fatal(err)
			}
			if _, err := data.ResolveQuestion(ctx, sid, q.ID, question.Reply{Status: "answered", Answers: [][]string{{"One", "Two"}}}); !errors.Is(err, question.ErrInvalid) {
				t.Fatal(err)
			}
			var commits atomic.Int32
			var wg sync.WaitGroup
			reply := question.Reply{Status: "answered", Answers: [][]string{{"custom answer"}}}
			for range 8 {
				wg.Go(func() {
					result, err := data.ResolveQuestion(ctx, sid, q.ID, reply)
					if err != nil {
						t.Error(err)
					}
					if result.Changed {
						commits.Add(1)
					}
				})
			}
			wg.Wait()
			if commits.Load() != 1 {
				t.Fatalf("commits=%d", commits.Load())
			}
			current, _ := data.GetSessionRun(ctx, sid, admission.Run.ID)
			if current.Status != store.SessionRunStatusAwaitingInput {
				t.Fatal(current.Status)
			}
			if _, err := data.ResolveQuestion(ctx, sid, q.ID, question.Reply{Status: "dismissed"}); !errors.Is(err, question.ErrConflict) {
				t.Fatal(err)
			}
			if _, err := data.ResolveQuestion(ctx, sid, second.ID, question.Reply{Status: "dismissed"}); err != nil {
				t.Fatal(err)
			}
			if duplicate, err := data.ResolveQuestion(ctx, sid, second.ID, question.Reply{Status: "dismissed", Answers: [][]string{}}); err != nil || duplicate.Changed {
				t.Fatalf("dismissal retry: %#v %v", duplicate, err)
			}
			claimed, err := data.ClaimNextSessionRun(ctx, sid)
			if err != nil || claimed.Run.ID != admission.Run.ID || claimed.Run.RecoveryAttempts != 0 {
				t.Fatalf("claim=%#v %v", claimed, err)
			}
			// An answer can win the race with suspension.
			early := create("que_early", admission.Run.ID)
			if _, err := data.ResolveQuestion(ctx, sid, early.ID, reply); err != nil {
				t.Fatal(err)
			}
			boundary, err := data.SuspendSessionRunForQuestions(ctx, sid, admission.Run.ID)
			if err != nil || boundary.Run.Status != store.SessionRunStatusQueued {
				t.Fatalf("early reply=%#v %v", boundary, err)
			}
			if _, err := data.ClaimNextSessionRun(ctx, sid); err != nil {
				t.Fatal(err)
			}
			cancelled := create("que_cancel", admission.Run.ID)
			if _, err := data.SuspendSessionRunForQuestions(ctx, sid, admission.Run.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := data.SettleSessionRun(ctx, store.SessionRunSettlement{ID: admission.Run.ID, ExpectedStatus: store.SessionRunStatusAwaitingInput, Status: store.SessionRunStatusAborted}); err != nil {
				t.Fatal(err)
			}
			if _, err := data.ResolveQuestion(ctx, sid, cancelled.ID, reply); !errors.Is(err, question.ErrConflict) {
				t.Fatal(err)
			}
			// Standalone questions do not require or suspend a run.
			standalone := create("que_standalone", "")
			if _, err := data.ResolveQuestion(ctx, sid, standalone.ID, reply); err != nil {
				t.Fatal(err)
			}
			events, err := data.(store.AggregateEventReader).ListAggregateEvents(ctx, store.AggregateRef{Type: store.AggregateSession, ID: sid}, 0, 1000)
			if err != nil {
				t.Fatal(err)
			}
			projection, err := store.ProjectSessionAggregate(events)
			if err != nil {
				t.Fatal(err)
			}
			questions, err := data.ListQuestions(ctx, sid)
			if err != nil || !reflect.DeepEqual(questions, projection.Questions) {
				t.Fatalf("question replay differs: %v", err)
			}
			if db, ok := data.(interface {
				RebuildSessionProjections(context.Context, string) error
			}); ok {
				if err := db.RebuildSessionProjections(ctx, sid); err != nil {
					t.Fatal(err)
				}
				rebuilt, err := data.ListQuestions(ctx, sid)
				if err != nil || !reflect.DeepEqual(questions, rebuilt) {
					t.Fatalf("rebuild differs: %v", err)
				}
			}
		})
	}
}
