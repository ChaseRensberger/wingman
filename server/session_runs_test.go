package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

type transientClaimStore struct {
	store.Store
	failures int
	attempts int
}

func (s *transientClaimStore) ClaimNextSessionRun(ctx context.Context, sessionID string) (store.SessionRunTransition, error) {
	s.attempts++
	if s.attempts <= s.failures {
		return store.SessionRunTransition{}, errors.New("temporary claim failure")
	}
	return s.Store.ClaimNextSessionRun(ctx, sessionID)
}

func TestSessionRunManagerRetriesTransientClaims(t *testing.T) {
	data := memory.NewStore()
	if err := data.CreateSession(&store.Session{ID: "ses_claim_retry"}); err != nil {
		t.Fatal(err)
	}
	if _, err := data.AdmitSessionRun(context.Background(), store.SessionRun{SessionID: "ses_claim_retry"}); err != nil {
		t.Fatal(err)
	}
	retrying := &transientClaimStore{Store: data, failures: 2}
	manager := newSessionRunManager(New(Config{Store: retrying}))
	transition, err := manager.claim(context.Background(), "ses_claim_retry")
	if err != nil {
		t.Fatal(err)
	}
	if !transition.Changed || transition.Run.Status != store.SessionRunStatusRunning || retrying.attempts != 3 {
		t.Fatalf("transition = %#v, attempts = %d", transition, retrying.attempts)
	}
}

func TestSessionRunAbortCancelsRunButNotWorker(t *testing.T) {
	manager := newSessionRunManager(New(Config{Store: memory.NewStore()}))
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	runCtx, cancelRun := context.WithCancel(workerCtx)
	defer cancelWorker()
	manager.active["ses_abort"] = cancelWorker
	manager.runCancel["ses_abort"] = cancelRun

	if got := manager.abort("ses_abort"); got != 1 {
		t.Fatalf("abort = %d, want 1", got)
	}
	if runCtx.Err() != context.Canceled {
		t.Fatalf("run context error = %v, want canceled", runCtx.Err())
	}
	if workerCtx.Err() != nil {
		t.Fatalf("worker context error = %v, want active", workerCtx.Err())
	}
}

func TestSessionRunReconcilerDiscoversQueuedWork(t *testing.T) {
	data := memory.NewStore()
	ctx := context.Background()
	if err := data.CreateSession(&store.Session{ID: "ses_reconcile"}); err != nil {
		t.Fatal(err)
	}
	admission, err := data.AdmitSessionRun(ctx, store.SessionRun{SessionID: "ses_reconcile", Message: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: data})
	server.runs.reconcileInterval = time.Millisecond
	server.runs.startReconciler()
	t.Cleanup(func() {
		server.shutdownCancel()
		server.runs.stop()
		waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.runs.wait(waitCtx)
	})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		run, err := data.GetSessionRun(ctx, admission.Run.SessionID, admission.Run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status == store.SessionRunStatusFailed {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("reconciler did not execute queued run")
}
