package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

type transientClaimStore struct {
	store.Store
	failures int
	attempts int
}

type transientSettlementStore struct {
	store.Store
	failures int
	attempts int
}

func TestPermissionRequestManagerResolvesAndRemembersGrant(t *testing.T) {
	data := memory.NewStore()
	if err := data.CreateSession(&store.Session{ID: "ses_permission"}); err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: data, PermissionTimeout: time.Second})
	prompter := server.permissionRequests.prompter("ses_permission", "")
	responses := make(chan struct {
		response run.PermissionResponse
		err      error
	}, 1)
	go func() {
		response, err := prompter.Request(context.Background(), run.PermissionRequestInfo{CallID: "call_one", Action: "edit", Resources: []string{"a.go"}})
		responses <- struct {
			response run.PermissionResponse
			err      error
		}{response, err}
	}()
	request := waitForPermissionRequest(t, data, "ses_permission")
	if request.CallID != "call_one" {
		t.Fatalf("request = %#v", request)
	}
	resolved, err := server.permissionRequests.resolve(context.Background(), request.SessionID, request.ID, store.PermissionRequestStatusApproved, store.PermissionResponseAlways, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != store.PermissionRequestStatusApproved {
		t.Fatalf("resolved = %#v", resolved)
	}
	result := <-responses
	if result.err != nil || result.response != run.PermissionResponseAlways {
		t.Fatalf("response = %#v", result)
	}
	if waiters := permissionWaiterCount(server.permissionRequests); waiters != 0 {
		t.Fatalf("waiters = %d, want 0", waiters)
	}
	response, err := prompter.Request(context.Background(), run.PermissionRequestInfo{Action: "edit", Resources: []string{"a.go"}})
	if err != nil || response != run.PermissionResponseAlways {
		t.Fatalf("remembered response = %q, %v", response, err)
	}
	requests, err := data.ListPermissionRequests(context.Background(), "ses_permission")
	if err != nil || len(requests) != 1 {
		t.Fatalf("requests = %#v, %v", requests, err)
	}
	events, err := data.ListSessionEvents(context.Background(), "ses_permission", 0, 10)
	if err != nil || len(events) != 2 {
		t.Fatalf("events = %#v, %v", events, err)
	}
}

func TestPermissionRequestManagerTimeoutCleansWaiter(t *testing.T) {
	data := memory.NewStore()
	if err := data.CreateSession(&store.Session{ID: "ses_permission_timeout"}); err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: data, PermissionTimeout: time.Millisecond})
	_, err := server.permissionRequests.prompter("ses_permission_timeout", "").Request(context.Background(), run.PermissionRequestInfo{Action: "edit", Resources: []string{"a.go"}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
	requests, err := data.ListPermissionRequests(context.Background(), "ses_permission_timeout")
	if err != nil || len(requests) != 1 || requests[0].Status != store.PermissionRequestStatusTimedOut {
		t.Fatalf("requests = %#v, %v", requests, err)
	}
	if waiters := permissionWaiterCount(server.permissionRequests); waiters != 0 {
		t.Fatalf("waiters = %d, want 0", waiters)
	}
}

type blockingPermissionResolutionStore struct{ store.Store }

func (s blockingPermissionResolutionStore) ResolvePermissionRequest(ctx context.Context, resolution store.PermissionRequestResolution) (store.PermissionRequestTransition, error) {
	<-ctx.Done()
	return store.PermissionRequestTransition{}, ctx.Err()
}

func TestPermissionRequestManagerBoundsResolutionPersistence(t *testing.T) {
	data := memory.NewStore()
	if err := data.CreateSession(&store.Session{ID: "ses_resolution_timeout"}); err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: blockingPermissionResolutionStore{Store: data}, PermissionTimeout: time.Millisecond})
	server.permissionRequests.resolutionTimeout = 10 * time.Millisecond
	result := make(chan error, 1)
	go func() {
		_, err := server.permissionRequests.prompter("ses_resolution_timeout", "").Request(context.Background(), run.PermissionRequestInfo{Action: "edit", Resources: []string{"a.go"}})
		result <- err
	}()
	_ = waitForPermissionRequest(t, data, "ses_resolution_timeout")
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not return after resolution timeout")
	}
	if waiters := permissionWaiterCount(server.permissionRequests); waiters != 0 {
		t.Fatalf("waiters = %d, want 0", waiters)
	}
}

func TestPermissionRequestManagerOnceRejectAndCancellation(t *testing.T) {
	for _, test := range []struct {
		name, response string
		status         string
		want           run.PermissionResponse
	}{
		{"once", store.PermissionResponseOnce, store.PermissionRequestStatusApproved, run.PermissionResponseOnce},
		{"reject", store.PermissionResponseReject, store.PermissionRequestStatusRejected, run.PermissionResponseReject},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := memory.NewStore()
			if err := data.CreateSession(&store.Session{ID: "ses_" + test.name}); err != nil {
				t.Fatal(err)
			}
			server := New(Config{Store: data, PermissionTimeout: time.Second})
			result := make(chan struct {
				response run.PermissionResponse
				err      error
			}, 1)
			go func() {
				response, err := server.permissionRequests.prompter("ses_"+test.name, "").Request(context.Background(), run.PermissionRequestInfo{Action: "edit", Resources: []string{"a.go"}})
				result <- struct {
					response run.PermissionResponse
					err      error
				}{response, err}
			}()
			request := waitForPermissionRequest(t, data, "ses_"+test.name)
			if _, err := server.permissionRequests.resolve(context.Background(), request.SessionID, request.ID, test.status, test.response, "", ""); err != nil {
				t.Fatal(err)
			}
			got := <-result
			if got.err != nil || got.response != test.want {
				t.Fatalf("result = %#v", got)
			}
			grants, err := data.ListPermissionGrants(context.Background(), request.SessionID)
			if err != nil || len(grants) != 0 {
				t.Fatalf("grants = %#v, %v", grants, err)
			}
			if waiters := permissionWaiterCount(server.permissionRequests); waiters != 0 {
				t.Fatalf("waiters = %d", waiters)
			}
		})
	}

	data := memory.NewStore()
	if err := data.CreateSession(&store.Session{ID: "ses_cancel"}); err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: data, PermissionTimeout: time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := server.permissionRequests.prompter("ses_cancel", "").Request(ctx, run.PermissionRequestInfo{Action: "edit", Resources: []string{"a.go"}})
		result <- err
	}()
	_ = waitForPermissionRequest(t, data, "ses_cancel")
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	requests, err := data.ListPermissionRequests(context.Background(), "ses_cancel")
	if err != nil || len(requests) != 1 || requests[0].Status != store.PermissionRequestStatusInterrupted {
		t.Fatalf("requests = %#v, %v", requests, err)
	}
	if waiters := permissionWaiterCount(server.permissionRequests); waiters != 0 {
		t.Fatalf("waiters = %d", waiters)
	}
}

func TestPermissionRequestManagerReplyRaceHasOneTerminalEvent(t *testing.T) {
	data := memory.NewStore()
	if err := data.CreateSession(&store.Session{ID: "ses_race"}); err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: data, PermissionTimeout: time.Millisecond})
	result := make(chan error, 1)
	go func() {
		_, err := server.permissionRequests.prompter("ses_race", "").Request(context.Background(), run.PermissionRequestInfo{Action: "edit", Resources: []string{"a.go"}})
		result <- err
	}()
	request := waitForPermissionRequest(t, data, "ses_race")
	replied := make(chan struct{})
	go func() {
		_, _ = server.permissionRequests.resolve(context.Background(), request.SessionID, request.ID, store.PermissionRequestStatusApproved, store.PermissionResponseOnce, "", "")
		close(replied)
	}()
	<-replied
	<-result
	events, err := data.ListSessionEvents(context.Background(), "ses_race", 0, 10)
	if err != nil || len(events) != 2 || events[0].Type != "session.permission.requested" || events[1].Type != "session.permission.resolved" {
		t.Fatalf("events = %#v, %v", events, err)
	}
	if waiters := permissionWaiterCount(server.permissionRequests); waiters != 0 {
		t.Fatalf("waiters = %d", waiters)
	}
}

func TestPermissionRequestManagerReplyCancellationRaceHasOneTerminalEvent(t *testing.T) {
	data := memory.NewStore()
	if err := data.CreateSession(&store.Session{ID: "ses_cancel_race"}); err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: data, PermissionTimeout: time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan struct {
		response run.PermissionResponse
		err      error
	}, 1)
	go func() {
		response, err := server.permissionRequests.prompter("ses_cancel_race", "").Request(ctx, run.PermissionRequestInfo{Action: "edit", Resources: []string{"a.go"}})
		result <- struct {
			response run.PermissionResponse
			err      error
		}{response, err}
	}()
	request := waitForPermissionRequest(t, data, "ses_cancel_race")
	done := make(chan struct{})
	go func() {
		_, _ = server.permissionRequests.resolve(context.Background(), request.SessionID, request.ID, store.PermissionRequestStatusApproved, store.PermissionResponseOnce, "", "")
		close(done)
	}()
	cancel()
	<-done
	got := <-result
	if got.response != run.PermissionResponseOnce && !errors.Is(got.err, context.Canceled) {
		t.Fatalf("result = %#v", got)
	}
	events, err := data.ListSessionEvents(context.Background(), "ses_cancel_race", 0, 10)
	if err != nil || len(events) != 2 || events[1].Type != "session.permission.resolved" {
		t.Fatalf("events = %#v, %v", events, err)
	}
	if waiters := permissionWaiterCount(server.permissionRequests); waiters != 0 {
		t.Fatalf("waiters = %d", waiters)
	}
}

func permissionWaiterCount(manager *permissionRequestManager) int {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return len(manager.waiters)
}

func waitForPermissionRequest(t *testing.T, data store.Store, sessionID string) store.PermissionRequest {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		requests, err := data.ListPermissionRequests(context.Background(), sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if len(requests) == 1 {
			return requests[0]
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("permission request was not created")
	return store.PermissionRequest{}
}

func (s *transientClaimStore) ClaimNextSessionRun(ctx context.Context, sessionID string) (store.SessionRunTransition, error) {
	s.attempts++
	if s.attempts <= s.failures {
		return store.SessionRunTransition{}, errors.New("temporary claim failure")
	}
	return s.Store.ClaimNextSessionRun(ctx, sessionID)
}

func (s *transientSettlementStore) SettleSessionRun(ctx context.Context, settlement store.SessionRunSettlement) (store.SessionRunTransition, error) {
	s.attempts++
	if s.attempts <= s.failures {
		return store.SessionRunTransition{}, errors.New("temporary settlement failure")
	}
	return s.Store.SettleSessionRun(ctx, settlement)
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

func TestSessionRunManagerRetriesTerminalSettlementBeforeNextClaim(t *testing.T) {
	data := memory.NewStore()
	ctx := context.Background()
	if err := data.CreateSession(&store.Session{ID: "ses_settlement_retry"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"run_settlement_first", "run_settlement_second"} {
		if _, err := data.AdmitSessionRun(ctx, store.SessionRun{ID: id, SessionID: "ses_settlement_retry"}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := data.ClaimNextSessionRun(ctx, "ses_settlement_retry")
	if err != nil || first.Run.ID != "run_settlement_first" {
		t.Fatalf("first claim = %#v, error = %v", first, err)
	}
	retrying := &transientSettlementStore{Store: data, failures: 1}
	manager := newSessionRunManager(New(Config{Store: retrying}))

	blocked, err := retrying.ClaimNextSessionRun(ctx, "ses_settlement_retry")
	if err != nil || blocked.Run.ID != "" {
		t.Fatalf("claim before settlement = %#v, error = %v", blocked, err)
	}
	if !manager.settle(ctx, store.SessionRunSettlement{ID: first.Run.ID, ExpectedStatus: store.SessionRunStatusRunning, Status: store.SessionRunStatusCompleted}) {
		t.Fatal("settlement was deferred, want committed retry")
	}
	if retrying.attempts != 2 {
		t.Fatalf("settlement attempts = %d, want 2", retrying.attempts)
	}
	settled, err := data.GetSessionRun(ctx, "ses_settlement_retry", first.Run.ID)
	if err != nil || settled.Status != store.SessionRunStatusCompleted {
		t.Fatalf("settled run = %#v, error = %v", settled, err)
	}
	events, err := data.ListSessionEvents(ctx, "ses_settlement_retry", 0, 10)
	if err != nil || len(events) != 4 || events[2].Type != "session.run.started" || events[3].Type != "session.run.completed" {
		t.Fatalf("events = %#v, error = %v", events, err)
	}
	next, err := manager.claim(ctx, "ses_settlement_retry")
	if err != nil || next.Run.ID != "run_settlement_second" {
		t.Fatalf("next claim = %#v, error = %v", next, err)
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
