package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestPermissionRequestLifecycleParity(t *testing.T) {
	for _, open := range []struct {
		name string
		open func(*testing.T) store.Store
	}{
		{"sqlite", func(t *testing.T) store.Store {
			data, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "wingman.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = data.Close() })
			return data
		}},
		{"memory", func(t *testing.T) store.Store { return memory.NewStore() }},
	} {
		t.Run(open.name, func(t *testing.T) {
			ctx, data := context.Background(), open.open(t)
			for _, id := range []string{"ses_one", "ses_other"} {
				if err := data.CreateSession(&store.Session{ID: id}); err != nil {
					t.Fatal(err)
				}
			}
			created, err := data.CreatePermissionRequest(ctx, store.PermissionRequest{ID: "prq_always", SessionID: "ses_one", Action: "filesystem.write", Resources: []string{"a", "b"}})
			if err != nil || !created.Changed || created.Event.Type != "session.permission.requested" || created.Event.Seq != 1 {
				t.Fatalf("create = %#v, %v", created, err)
			}
			duplicate, err := data.CreatePermissionRequest(ctx, store.PermissionRequest{ID: "prq_always", SessionID: "ses_one", Action: "filesystem.write", Resources: []string{"a", "b"}})
			if err != nil || duplicate.Changed || duplicate.Event.ID != "" || duplicate.Request.ID != created.Request.ID {
				t.Fatalf("idempotent create = %#v, %v", duplicate, err)
			}
			if watermark, err := data.SessionEventWatermark(ctx, "ses_one"); err != nil || watermark != 1 {
				t.Fatalf("watermark after idempotent create = %d, %v; want 1", watermark, err)
			}
			if _, err := data.CreatePermissionRequest(ctx, store.PermissionRequest{ID: "prq_always", SessionID: "ses_one", Action: "filesystem.write", Resources: []string{"b", "a"}}); !errors.Is(err, store.ErrPermissionRequestTransitionConflict) {
				t.Fatalf("conflicting create error = %v", err)
			}
			resolved, err := data.ResolvePermissionRequest(ctx, store.PermissionRequestResolution{SessionID: "ses_one", RequestID: created.Request.ID, Status: store.PermissionRequestStatusApproved, Response: store.PermissionResponseAlways})
			if err != nil || !resolved.Changed || resolved.Event.Type != "session.permission.resolved" || resolved.Event.Seq != 2 {
				t.Fatalf("resolve = %#v, %v", resolved, err)
			}
			grants, err := data.ListPermissionGrants(ctx, "ses_one")
			if err != nil || len(grants) != 2 {
				t.Fatalf("grants = %#v, %v; want two", grants, err)
			}
			reader, ok := data.(store.AggregateEventReader)
			if !ok {
				t.Fatal("store does not expose aggregate history")
			}
			events, err := reader.ListAggregateEvents(ctx, store.AggregateRef{Type: store.AggregateSession, ID: "ses_one"}, 0, 100)
			if err != nil {
				t.Fatal(err)
			}
			replayedRequests, err := store.ProjectSessionPermissionRequests(events)
			if err != nil {
				t.Fatal(err)
			}
			storedRequests, err := data.ListPermissionRequests(ctx, "ses_one")
			if err != nil || !reflect.DeepEqual(replayedRequests, storedRequests) {
				t.Fatalf("replayed requests = %#v, stored = %#v, error = %v", replayedRequests, storedRequests, err)
			}
			replayedGrants, err := store.ProjectSessionPermissionGrants(events)
			if err != nil || !reflect.DeepEqual(replayedGrants, grants) {
				t.Fatalf("replayed grants = %#v, stored = %#v, error = %v", replayedGrants, grants, err)
			}
			session, err := data.GetSession("ses_one")
			if err != nil || session.AggregateVersion != 5 {
				t.Fatalf("session version = %#v, error = %v; want 5", session, err)
			}
			idempotent, err := data.ResolvePermissionRequest(ctx, store.PermissionRequestResolution{SessionID: "ses_one", RequestID: created.Request.ID, Status: store.PermissionRequestStatusApproved, Response: store.PermissionResponseAlways})
			if err != nil || idempotent.Changed || idempotent.Event.ID != "" {
				t.Fatalf("idempotent resolve = %#v, %v", idempotent, err)
			}
			if _, err := data.ResolvePermissionRequest(ctx, store.PermissionRequestResolution{SessionID: "ses_one", RequestID: created.Request.ID, Status: store.PermissionRequestStatusRejected, Response: store.PermissionResponseReject}); !errors.Is(err, store.ErrPermissionRequestTransitionConflict) {
				t.Fatalf("terminal rewrite error = %v", err)
			}
			if _, err := data.GetPermissionRequest(ctx, "ses_other", created.Request.ID); !errors.Is(err, store.ErrPermissionRequestNotFound) {
				t.Fatalf("cross-session request error = %v", err)
			}

			once, err := data.CreatePermissionRequest(ctx, store.PermissionRequest{ID: "prq_once", SessionID: "ses_one", Action: "filesystem.write", Resources: []string{"c"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := data.ResolvePermissionRequest(ctx, store.PermissionRequestResolution{SessionID: "ses_one", RequestID: once.Request.ID, Status: store.PermissionRequestStatusApproved, Response: store.PermissionResponseOnce}); err != nil {
				t.Fatal(err)
			}
			grants, err = data.ListPermissionGrants(ctx, "ses_one")
			if err != nil || len(grants) != 2 {
				t.Fatalf("once grants = %#v, %v", grants, err)
			}

			pending, err := data.CreatePermissionRequest(ctx, store.PermissionRequest{ID: "prq_pending", SessionID: "ses_one", Action: "shell.exec", Resources: []string{"pwd"}})
			if err != nil {
				t.Fatal(err)
			}
			interrupted, err := data.InterruptPendingPermissionRequests(ctx)
			if err != nil || len(interrupted) != 1 || interrupted[0].Request.ID != pending.Request.ID || interrupted[0].Request.Status != store.PermissionRequestStatusInterrupted || interrupted[0].Event.Type != "session.permission.resolved" {
				t.Fatalf("interrupt = %#v, %v", interrupted, err)
			}
			foreign, err := data.AdmitSessionRun(ctx, store.SessionRun{SessionID: "ses_other"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := data.CreatePermissionRequest(ctx, store.PermissionRequest{SessionID: "ses_one", RunID: foreign.Run.ID, Action: "shell.exec", Resources: []string{"pwd"}}); err == nil {
				t.Fatal("cross-session run ownership was accepted")
			}
			toolRun, err := data.AdmitSessionRun(ctx, store.SessionRun{SessionID: "ses_one"})
			if err != nil {
				t.Fatal(err)
			}
			if err := data.SaveToolUse(ctx, store.ToolUse{ID: "tlu_permission", SessionID: "ses_one", RunID: toolRun.Run.ID, Step: 1, Ordinal: 0, CallID: "call_expected", Name: "shell", Status: store.ToolUseStatusProposed}); err != nil {
				t.Fatal(err)
			}
			otherRun, err := data.AdmitSessionRun(ctx, store.SessionRun{SessionID: "ses_one"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := data.CreatePermissionRequest(ctx, store.PermissionRequest{SessionID: "ses_one", RunID: otherRun.Run.ID, ToolUseID: "tlu_permission", CallID: "call_expected", Action: "shell.exec", Resources: []string{"pwd"}}); err == nil {
				t.Fatal("tool use/run mismatch was accepted")
			}
			if _, err := data.CreatePermissionRequest(ctx, store.PermissionRequest{SessionID: "ses_one", RunID: toolRun.Run.ID, ToolUseID: "tlu_permission", CallID: "call_wrong", Action: "shell.exec", Resources: []string{"pwd"}}); err == nil {
				t.Fatal("tool use/call mismatch was accepted")
			}
			if _, err := data.CreatePermissionRequest(ctx, store.PermissionRequest{SessionID: "ses_one", RunID: toolRun.Run.ID, ToolUseID: "tlu_permission", CallID: "call_expected", Action: "shell.exec", Resources: []string{"pwd"}}); err != nil {
				t.Fatalf("matching tool use ownership = %v", err)
			}
			session, err = data.GetSession("ses_one")
			if err != nil {
				t.Fatal(err)
			}
			if err := data.PurgeSession(ctx, "ses_one", session.AggregateVersion); err != nil {
				t.Fatal(err)
			}
			if _, err := data.ListPermissionGrants(ctx, "ses_one"); !errors.Is(err, store.ErrSessionNotFound) {
				t.Fatalf("purged grants error = %v", err)
			}
		})
	}
}

func TestPermissionRequestConcurrentResolution(t *testing.T) {
	for _, opener := range permissionResolutionStores() {
		t.Run(opener.name, func(t *testing.T) {
			first, second := opener.open(t)
			runConcurrentPermissionResolution(t, first, second, false)
		})
	}
}

func TestPermissionRequestConcurrentConflictingResolution(t *testing.T) {
	for _, opener := range permissionResolutionStores() {
		t.Run(opener.name, func(t *testing.T) {
			first, second := opener.open(t)
			runConcurrentPermissionResolution(t, first, second, true)
		})
	}
}

type permissionResolutionStore struct {
	name string
	open func(*testing.T) (store.Store, store.Store)
}

func permissionResolutionStores() []permissionResolutionStore {
	return []permissionResolutionStore{
		{"sqlite", func(t *testing.T) (store.Store, store.Store) {
			path := filepath.Join(t.TempDir(), "wingman.db")
			first, err := store.NewSQLiteStore(path)
			if err != nil {
				t.Fatal(err)
			}
			second, err := store.NewSQLiteStore(path)
			if err != nil {
				_ = first.Close()
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = first.Close(); _ = second.Close() })
			return first, second
		}},
		{"memory", func(t *testing.T) (store.Store, store.Store) {
			data := memory.NewStore()
			return data, data
		}},
	}
}

func runConcurrentPermissionResolution(t *testing.T, first, second store.Store, conflict bool) {
	t.Helper()
	ctx := context.Background()
	if err := first.CreateSession(&store.Session{ID: "ses_race"}); err != nil {
		t.Fatal(err)
	}
	request, err := first.CreatePermissionRequest(ctx, store.PermissionRequest{SessionID: "ses_race", Action: "shell.exec", Resources: []string{"pwd"}})
	if err != nil {
		t.Fatal(err)
	}
	resolutions := []store.PermissionRequestResolution{
		{SessionID: "ses_race", RequestID: request.Request.ID, Status: store.PermissionRequestStatusApproved, Response: store.PermissionResponseOnce},
		{SessionID: "ses_race", RequestID: request.Request.ID, Status: store.PermissionRequestStatusApproved, Response: store.PermissionResponseOnce},
	}
	if conflict {
		resolutions[1] = store.PermissionRequestResolution{SessionID: "ses_race", RequestID: request.Request.ID, Status: store.PermissionRequestStatusRejected, Response: store.PermissionResponseReject}
	}
	var wg sync.WaitGroup
	results := make(chan store.PermissionRequestTransition, 2)
	errs := make(chan error, 2)
	for i, data := range []store.Store{first, second} {
		wg.Add(1)
		go func(data store.Store, resolution store.PermissionRequestResolution) {
			defer wg.Done()
			transition, err := data.ResolvePermissionRequest(ctx, resolution)
			results <- transition
			errs <- err
		}(data, resolutions[i])
	}
	wg.Wait()
	close(results)
	close(errs)
	changed, conflicts := 0, 0
	for transition := range results {
		if transition.Changed {
			changed++
		}
	}
	for err := range errs {
		if err != nil {
			if !conflict || !errors.Is(err, store.ErrPermissionRequestTransitionConflict) {
				t.Fatalf("concurrent resolution error = %v", err)
			}
			conflicts++
		}
	}
	if changed != 1 {
		t.Fatalf("changed resolutions = %d, want 1", changed)
	}
	if conflict && conflicts != 1 {
		t.Fatalf("conflicting resolutions = %d, want 1", conflicts)
	}
}
