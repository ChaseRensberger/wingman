package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

type pausedClaimStore struct {
	store.Store
	claimed chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *pausedClaimStore) ClaimNextSessionRun(ctx context.Context, id string) (store.SessionRunTransition, error) {
	result, err := s.Store.ClaimNextSessionRun(ctx, id)
	if result.Changed {
		s.once.Do(func() {
			close(s.claimed)
			select {
			case <-s.release:
			case <-ctx.Done():
			}
		})
	}
	return result, err
}

func TestCancelClaimedRunBeforeWorkerRegistration(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			var storage store.Store = memory.NewStore()
			if backend == "sqlite" {
				db, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "cancellation.db"))
				if err != nil {
					t.Fatal(err)
				}
				storage = db
			}
			defer storage.Close()
			data := &pausedClaimStore{Store: storage, claimed: make(chan struct{}), release: make(chan struct{})}
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer upstream.Close()
			server := New(Config{Store: data})
			defer server.Close(context.Background())
			if err := data.CreateSession(&store.Session{ID: "ses_cancel_claim"}); err != nil {
				t.Fatal(err)
			}
			agent := store.Agent{ID: "agt_cancel_claim", ModelRef: "test/model", Options: map[string]any{agentOptionModelRoute: models.ModelInfo{Provider: "test", ID: "model", API: models.APIOpenAICompatible, BaseURL: upstream.URL}}}
			var runs []store.SessionRun
			for _, message := range []string{"cancel this", "run this next"} {
				admitted, err := data.AdmitSessionRun(ctx, store.SessionRun{SessionID: "ses_cancel_claim", Message: message, Agent: agent})
				if err != nil {
					t.Fatal(err)
				}
				runs = append(runs, admitted.Run)
			}
			server.runs.wake("ses_cancel_claim")
			select {
			case <-data.claimed:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			aborted, err := server.runs.abort(ctx, "ses_cancel_claim", runs[0].ID)
			close(data.release)
			if err != nil || aborted != 1 {
				t.Fatalf("abort=%d err=%v", aborted, err)
			}
			if err := server.runs.wait(ctx); err != nil {
				t.Fatal(err)
			}
			for i, want := range []string{store.SessionRunStatusAborted, store.SessionRunStatusCompleted} {
				current, err := data.GetSessionRun(ctx, "ses_cancel_claim", runs[i].ID)
				if err != nil || current.Status != want {
					t.Fatalf("run=%#v err=%v want=%s", current, err, want)
				}
			}
			messages, err := data.QueryMessages(ctx, "ses_cancel_claim", store.MessageQuery{RunID: runs[0].ID})
			if err != nil || len(messages) != 0 {
				t.Fatalf("canceled run messages=%#v err=%v", messages, err)
			}
			if calls.Load() != 1 {
				t.Fatalf("provider calls=%d; only the next run should execute", calls.Load())
			}
		})
	}
}
