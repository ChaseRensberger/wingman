package store_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestSessionSwitchesFollowStartedTurns(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "switches.db")
			var data store.Store = memory.NewStore()
			if backend == "sqlite" {
				db, err := store.NewSQLiteStore(path)
				if err != nil {
					t.Fatal(err)
				}
				data = db
			}
			defer func() { _ = data.Close() }()
			ctx := t.Context()
			if err := data.CreateSession(&store.Session{ID: "s"}); err != nil {
				t.Fatal(err)
			}
			plan := store.Agent{ID: "plan", Name: "Plan", ModelRef: "test/one"}
			build := store.Agent{ID: "build", Name: "Build", ModelRef: "test/two"}
			for i, agent := range []store.Agent{plan, {ID: "cancelled", ModelRef: "ignored"}, build, build} {
				id := []string{"first", "cancelled", "second", "third"}[i]
				if _, err := data.AdmitSessionRun(ctx, store.SessionRun{ID: id, SessionID: "s", Message: "same", Agent: agent}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := data.SettleSessionRun(ctx, store.SessionRunSettlement{ID: "cancelled", ExpectedStatus: "queued", Status: "aborted"}); err != nil {
				t.Fatal(err)
			}
			first, err := data.ClaimNextSessionRun(ctx, "s")
			if err != nil || len(first.Notices) != 0 {
				t.Fatalf("first = %#v, %v", first, err)
			}
			if notices, err := data.ListSessionNotices(ctx, "s"); err != nil || len(notices) != 0 {
				t.Fatalf("queued switch appeared early: %#v, %v", notices, err)
			}
			if _, err := data.SettleSessionRun(ctx, store.SessionRunSettlement{ID: "first", ExpectedStatus: "running", Status: "completed"}); err != nil {
				t.Fatal(err)
			}
			second, err := data.ClaimNextSessionRun(ctx, "s")
			if err != nil || second.Run.ID != "second" || len(second.Notices) != 2 {
				t.Fatalf("second = %#v, %v", second, err)
			}
			for i, want := range []string{store.EventSessionAgentSwitched, store.EventSessionModelSwitched} {
				event := second.Notices[i]
				if event.Type != want || event.Seq != second.Event.Seq+int64(i)+1 {
					t.Fatalf("notice = %#v", event)
				}
				var payload map[string]string
				if err := json.Unmarshal(event.DataJSON, &payload); err != nil {
					t.Fatal(err)
				}
				if payload["run_id"] != "second" || payload["previous"] != []string{"plan", "test/one"}[i] {
					t.Fatalf("payload = %#v", payload)
				}
			}
			if _, err := data.RequeueSessionRun(ctx, "second"); err != nil {
				t.Fatal(err)
			}
			resumed, err := data.ClaimNextSessionRun(ctx, "s")
			if err != nil || len(resumed.Notices) != 0 {
				t.Fatalf("recovery repeated notices: %#v, %v", resumed, err)
			}
			if _, err := data.SettleSessionRun(ctx, store.SessionRunSettlement{ID: "second", ExpectedStatus: "running", Status: "completed"}); err != nil {
				t.Fatal(err)
			}
			third, err := data.ClaimNextSessionRun(ctx, "s")
			if err != nil || len(third.Notices) != 0 {
				t.Fatalf("unchanged turn = %#v, %v", third, err)
			}
			notices, err := data.ListSessionNotices(ctx, "s")
			if err != nil || len(notices) != 2 {
				t.Fatalf("saved notices = %#v, %v", notices, err)
			}
			if backend == "sqlite" {
				if err := data.Close(); err != nil {
					t.Fatal(err)
				}
				data, err = store.NewSQLiteStore(path)
				if err != nil {
					t.Fatal(err)
				}
				restored, err := data.ListSessionNotices(ctx, "s")
				if err != nil || len(restored) != 2 || restored[0].ID != notices[0].ID {
					t.Fatalf("restored notices = %#v, %v", restored, err)
				}
			}
		})
	}
}
