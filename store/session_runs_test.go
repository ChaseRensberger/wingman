package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

func TestSessionRunsClaimInSequence(t *testing.T) {
	data, err := NewSQLiteStore(filepath.Join(t.TempDir(), "wingman.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	if err := data.CreateSession(&Session{ID: "ses_test"}); err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{"first", "second"} {
		if _, err := data.AdmitSessionRun(context.Background(), SessionRun{
			SessionID: "ses_test",
			Message:   message,
			Agent:     Agent{ID: "agt_test", ModelRef: "openai/gpt-5.6-terra"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []string{"first", "second"} {
		run, err := data.ClaimNextSessionRun(context.Background(), "ses_test")
		if err != nil {
			t.Fatal(err)
		}
		if run == nil || run.Message != want || run.Status != SessionRunStatusRunning {
			t.Fatalf("claimed run = %#v, want running %q", run, want)
		}
		if err := data.CompleteSessionRun(context.Background(), run.ID, SessionRunStatusCompleted, ""); err != nil {
			t.Fatal(err)
		}
	}
	run, err := data.ClaimNextSessionRun(context.Background(), "ses_test")
	if err != nil {
		t.Fatal(err)
	}
	if run != nil {
		t.Fatalf("claimed unexpected run: %#v", run)
	}
}

func TestAdmitSessionRunIsAtomicAndIdempotent(t *testing.T) {
	data := newTestSQLiteStore(t)
	session := &Session{ID: "ses_admit", WorkDir: "/workspace"}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	admission, err := data.AdmitSessionRun(context.Background(), SessionRun{SessionID: session.ID, RequestID: "request-1", Message: "hello", Agent: Agent{ID: "agt_test", CreatedAt: "2026-07-30T12:00:00Z", UpdatedAt: "2026-07-30T12:00:00Z"}, OutputSchemaJSON: []byte(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if !admission.Created || admission.SessionVersion != 2 || admission.Run.AdmittedVersion != 2 || admission.Run.Sequence != 1 {
		t.Fatalf("admission = %#v", admission)
	}
	if admission.QueuedEvent.Type != "session.run.queued" || admission.QueuedEvent.Seq != 1 || string(admission.QueuedEvent.DataJSON) != `{"run_id":"`+admission.Run.ID+`","message":"hello"}` {
		t.Fatalf("queued event = %#v", admission.QueuedEvent)
	}
	events, err := data.ListAggregateEvents(context.Background(), AggregateRef{Type: AggregateSession, ID: session.ID}, 0, 10)
	if err != nil || len(events) != 2 || events[1].Type != EventSessionRunAdmitted {
		t.Fatalf("aggregate events = %#v, error = %v", events, err)
	}
	projected, err := ProjectSessionRunAdmission(events[1])
	if err != nil || projected.ID != admission.Run.ID || string(projected.OutputSchemaJSON) != `{"type":"object"}` {
		t.Fatalf("projected run = %#v, error = %v", projected, err)
	}
	retry, err := data.AdmitSessionRun(context.Background(), SessionRun{SessionID: session.ID, RequestID: "request-1", Message: "hello", Agent: Agent{ID: "agt_test", CreatedAt: "2026-07-30T12:00:00Z", UpdatedAt: "2026-07-30T13:00:00Z"}, OutputSchemaJSON: []byte(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if retry.Created || retry.Run.ID != admission.Run.ID || retry.SessionVersion != 2 || retry.QueuedEvent.ID != "" {
		t.Fatalf("retry = %#v", retry)
	}
	if _, err := data.AdmitSessionRun(context.Background(), SessionRun{SessionID: session.ID, RequestID: "request-1", Message: "different", Agent: Agent{ID: "agt_test"}}); !errors.Is(err, ErrSessionRunAdmissionConflict) {
		t.Fatalf("conflicting retry error = %v", err)
	}
	stored, err := data.GetSession(session.ID)
	if err != nil || stored.AggregateVersion != 2 {
		t.Fatalf("session = %#v, error = %v", stored, err)
	}
	if queued, err := data.ListSessionEvents(context.Background(), session.ID, 0, 10); err != nil || len(queued) != 1 {
		t.Fatalf("queued events = %#v, error = %v", queued, err)
	}
	first, err := data.AdmitSessionRun(context.Background(), SessionRun{SessionID: session.ID, Message: "no key", Agent: Agent{ID: "agt_test"}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := data.AdmitSessionRun(context.Background(), SessionRun{SessionID: session.ID, Message: "no key", Agent: Agent{ID: "agt_test"}})
	if err != nil || first.Run.ID == second.Run.ID || second.Run.Sequence != 3 {
		t.Fatalf("unkeyed admissions = %#v, %#v, error = %v", first, second, err)
	}
}

func TestAdmitSessionRunRollsBackOnProjectionUpdateFailure(t *testing.T) {
	data := newTestSQLiteStore(t)
	if err := data.CreateSession(&Session{ID: "ses_admit_rollback"}); err != nil {
		t.Fatal(err)
	}
	if _, err := data.db.Exec(`CREATE TRIGGER fail_admission BEFORE UPDATE OF aggregate_version ON sessions WHEN OLD.id = 'ses_admit_rollback' BEGIN SELECT RAISE(ABORT, 'forced admission failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := data.AdmitSessionRun(context.Background(), SessionRun{SessionID: "ses_admit_rollback", RequestID: "request", Message: "hello"}); err == nil {
		t.Fatal("admission succeeded")
	}
	for _, query := range []string{`SELECT COUNT(*) FROM session_runs WHERE session_id = 'ses_admit_rollback'`, `SELECT COUNT(*) FROM aggregate_events WHERE aggregate_id = 'ses_admit_rollback' AND event_type = 'session.run.admitted'`, `SELECT COUNT(*) FROM session_events WHERE session_id = 'ses_admit_rollback'`} {
		var count int
		if err := data.db.QueryRow(query).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%q count = %d, error = %v", query, count, err)
		}
	}
}

func TestAdmitSessionRunConcurrentRetryAcrossHandles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wingman.db")
	first, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	if err := first.CreateSession(&Session{ID: "ses_admit_concurrent"}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan SessionRunAdmission, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, data := range []*SQLiteStore{first, second} {
		wg.Add(1)
		go func(data *SQLiteStore) {
			defer wg.Done()
			<-start
			result, err := data.AdmitSessionRun(context.Background(), SessionRun{SessionID: "ses_admit_concurrent", RequestID: "request", Message: "hello"})
			results <- result
			errs <- err
		}(data)
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	var created int
	var id string
	for result := range results {
		if result.Created {
			created++
		}
		if id == "" {
			id = result.Run.ID
		} else if id != result.Run.ID {
			t.Fatalf("run IDs differ: %q and %q", id, result.Run.ID)
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}
}

func TestAdmitSessionRunSnapshotsPlacement(t *testing.T) {
	data := newTestSQLiteStore(t)
	ctx := context.Background()
	session := &Session{ID: "ses_admit_placement", WorkDir: "/before"}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	first, err := data.AdmitSessionRun(ctx, SessionRun{SessionID: session.ID, RequestID: "request", Message: "hello", Agent: Agent{ID: "agt"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.MoveSession(ctx, session.ID, "/after", "", first.SessionVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := data.AdmitSessionRun(ctx, SessionRun{SessionID: session.ID, RequestID: "request", Message: "hello", Agent: Agent{ID: "agt"}}); !errors.Is(err, ErrSessionRunAdmissionConflict) {
		t.Fatalf("retry after move error = %v, want admission conflict", err)
	}
	claimed, err := data.ClaimNextSessionRun(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.ID != first.Run.ID || claimed.WorkDir != "/before" {
		t.Fatalf("claimed run = %#v, want original placement", claimed)
	}
}
