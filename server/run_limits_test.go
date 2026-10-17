package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

type blockedClaimStore struct {
	store.Store
	release <-chan struct{}
	claims  atomic.Int32
}

func (s *blockedClaimStore) ClaimNextSessionRun(ctx context.Context, _ string) (store.SessionRunTransition, error) {
	s.claims.Add(1)
	select {
	case <-s.release:
		return store.SessionRunTransition{}, nil
	case <-ctx.Done():
		return store.SessionRunTransition{}, ctx.Err()
	}
}

func TestRunWorkersRemainBoundedUnderLoad(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		data := &blockedClaimStore{Store: memory.NewStore(), release: release}
		s := New(Config{Store: data, MaxActiveRuns: 4})
		defer s.Close(context.Background())
		for i := range 200 {
			s.runs.wake(fmt.Sprintf("ses_%d", i))
		}
		synctest.Wait()
		if data.claims.Load() != 4 || s.runs.activeCount() != 4 {
			t.Fatalf("claims=%d active=%d", data.claims.Load(), s.runs.activeCount())
		}
		close(release)
		if err := s.runs.wait(t.Context()); err != nil {
			t.Fatal(err)
		}
		if data.claims.Load() != 200 || s.runs.activeCount() != 0 {
			t.Fatalf("claims=%d active=%d", data.claims.Load(), s.runs.activeCount())
		}
	})
}

func TestRunLimitFairnessCancellationAndRecovery(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			var data store.Store = memory.NewStore()
			if backend == "sqlite" {
				sql, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "limits.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer sql.Close()
				data = sql
			}
			started := make(chan struct{}, 8)
			release := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				started <- struct{}{}
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer upstream.Close()
			agent := store.Agent{ID: "agt_limits", ModelRef: "test/model", Options: map[string]any{agentOptionModelRoute: models.ModelInfo{Provider: "test", ID: "model", API: models.APIOpenAICompatible, BaseURL: upstream.URL}}}
			admit := func(id string) store.SessionRun {
				t.Helper()
				a, err := data.AdmitSessionRun(ctx, store.SessionRun{SessionID: id, Agent: agent, Message: "hello"})
				if err != nil {
					t.Fatal(err)
				}
				return a.Run
			}
			for _, id := range []string{"ses_a", "ses_b", "ses_c"} {
				if err := data.CreateSession(&store.Session{ID: id}); err != nil {
					t.Fatal(err)
				}
			}
			a1, a2, b, c := admit("ses_a"), admit("ses_a"), admit("ses_b"), admit("ses_c")
			s := New(Config{Store: data, MaxActiveRuns: 1})
			defer s.Close(context.Background())
			await := func() {
				t.Helper()
				select {
				case <-started:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			status := func(run store.SessionRun, want string) {
				t.Helper()
				got, err := data.GetSessionRun(ctx, run.SessionID, run.ID)
				if err != nil || got.Status != want {
					t.Fatalf("run=%s got=%#v err=%v want=%s", run.ID, got, err, want)
				}
			}
			s.runs.wake("ses_a")
			await()
			s.runs.wake("ses_b")
			s.runs.wake("ses_c")
			status(a1, store.SessionRunStatusRunning)
			status(a2, store.SessionRunStatusQueued)
			status(b, store.SessionRunStatusQueued)
			if s.runs.activeCount() != 1 {
				t.Fatal("worker limit exceeded")
			}
			if _, err := data.SettleSessionRun(ctx, store.SessionRunSettlement{ID: c.ID, ExpectedStatus: store.SessionRunStatusQueued, Status: store.SessionRunStatusAborted}); err != nil {
				t.Fatal(err)
			}
			release <- struct{}{}
			await()
			status(b, store.SessionRunStatusRunning)
			status(a2, store.SessionRunStatusQueued)
			// A shutdown leaves both the active run and the waiting run recoverable.
			if err := s.Close(ctx); err != nil {
				t.Fatal(err)
			}
			status(a2, store.SessionRunStatusQueued)
			status(c, store.SessionRunStatusAborted)
			s2 := New(Config{Store: data, MaxActiveRuns: 1})
			defer s2.Close(context.Background())
			if err := s2.Start(ctx); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				await()
				if s2.runs.activeCount() != 1 {
					t.Fatal("recovery exceeded limit")
				}
				release <- struct{}{}
			}
			for {
				ar, _ := data.GetSessionRun(ctx, a2.SessionID, a2.ID)
				br, _ := data.GetSessionRun(ctx, b.SessionID, b.ID)
				if ar.Status == store.SessionRunStatusCompleted && br.Status == store.SessionRunStatusCompleted {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(time.Millisecond):
				}
			}
			status(c, store.SessionRunStatusAborted)
			select {
			case <-started:
				t.Fatal("canceled run dispatched")
			default:
			}
		})
	}
}
