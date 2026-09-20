package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/trigger"
)

func storedTrigger(t *testing.T, data *SQLiteStore) trigger.Trigger {
	t.Helper()
	client, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	next := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	item, err := data.SaveTrigger(t.Context(), trigger.Trigger{
		Config:   trigger.Config{Name: "Morning review", Source: trigger.Source{Type: "cron", Expression: "0 8 * * *", TimeZone: "UTC"}, Target: trigger.Target{AgentID: "agt_test"}, Prompt: "Review", Enabled: true},
		ClientID: client.ID, NextFireAt: &next,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func triggerSubmission(item trigger.Trigger) TriggerSubmission {
	return TriggerSubmission{
		Trigger:    item,
		Occurrence: trigger.Occurrence{Source: "cron", RequestID: item.NextFireAt.Format(time.RFC3339), ScheduledAt: *item.NextFireAt},
		NextFireAt: item.NextFireAt.Add(24 * time.Hour),
		Session:    Session{Title: item.Name, ClientID: item.ClientID},
		Run:        SessionRun{Message: item.Prompt, Agent: Agent{ID: "agt_test"}},
	}
}

func TestTriggerAdmissionAtomicRetryAndOverlap(t *testing.T) {
	data := newTestSQLiteStore(t)
	item := storedTrigger(t, data)
	submission := triggerSubmission(item)
	first, err := data.CommitTriggerOccurrence(t.Context(), submission)
	if err != nil || !first.Admission.Created || first.Occurrence.Status != "queued" {
		t.Fatalf("first = %#v, %v", first, err)
	}
	second, err := data.CommitTriggerOccurrence(t.Context(), submission)
	if err != nil || second.Admission.Created || second.Occurrence.ID != first.Occurrence.ID {
		t.Fatalf("retry = %#v, %v", second, err)
	}
	updated, err := data.GetTrigger(t.Context(), item.ClientID, item.ID)
	if err != nil || !updated.NextFireAt.Equal(submission.NextFireAt) || updated.RunCount != 1 {
		t.Fatalf("updated = %#v, %v", updated, err)
	}
	manual := submission
	manual.Occurrence.Source, manual.Occurrence.RequestID = "manual", "manual-1"
	busy, err := data.CommitTriggerOccurrence(t.Context(), manual)
	if err != nil || busy.Occurrence.Status != "skipped" || busy.Occurrence.SessionID != "" {
		t.Fatalf("overlap = %#v, %v", busy, err)
	}
	if _, err := data.SettleSessionRun(t.Context(), SessionRunSettlement{ID: first.Admission.Run.ID, ExpectedStatus: SessionRunStatusQueued, Status: SessionRunStatusAborted}); err != nil {
		t.Fatal(err)
	}
	manual.Occurrence.RequestID = "manual-2"
	fresh, err := data.CommitTriggerOccurrence(t.Context(), manual)
	if err != nil || fresh.Occurrence.SessionID == first.Occurrence.SessionID || !fresh.Admission.Created {
		t.Fatalf("fresh = %#v, %v", fresh, err)
	}
	updated, err = data.GetTrigger(t.Context(), item.ClientID, item.ID)
	if err != nil || !updated.NextFireAt.Equal(submission.NextFireAt) {
		t.Fatalf("manual changed schedule: %#v, %v", updated, err)
	}
}

func TestTriggerAdmissionRollsBackEveryRecord(t *testing.T) {
	data := newTestSQLiteStore(t)
	item := storedTrigger(t, data)
	if _, err := data.db.Exec(`CREATE TRIGGER reject_occurrence BEFORE INSERT ON trigger_occurrences BEGIN SELECT RAISE(ABORT, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := data.CommitTriggerOccurrence(t.Context(), triggerSubmission(item)); err == nil {
		t.Fatal("expected failure")
	}
	for _, table := range []string{"sessions", "session_runs", "session_events", "aggregate_events", "trigger_occurrences"} {
		var count int
		if err := data.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s = %d, %v", table, count, err)
		}
	}
	unchanged, err := data.GetTrigger(t.Context(), item.ClientID, item.ID)
	if err != nil || !unchanged.NextFireAt.Equal(*item.NextFireAt) {
		t.Fatalf("cursor advanced: %#v, %v", unchanged, err)
	}
}

func TestTriggerConcurrentAdmissionAcrossHandlesAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "triggers.db")
	first, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	item := storedTrigger(t, first)
	var wg sync.WaitGroup
	results := make(chan TriggerSubmissionResult, 12)
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data := first
			if i%2 == 1 {
				data = second
			}
			result, err := data.CommitTriggerOccurrence(context.Background(), triggerSubmission(item))
			if err != nil {
				t.Error(err)
				return
			}
			results <- result
		}()
	}
	wg.Wait()
	close(results)
	created := 0
	for result := range results {
		if result.Admission.Created {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("admitted %d runs", created)
	}
	reopened, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	retry, err := reopened.CommitTriggerOccurrence(t.Context(), triggerSubmission(item))
	if err != nil || retry.Admission.Created || retry.Occurrence.RunID == "" {
		t.Fatalf("restart retry = %#v, %v", retry, err)
	}
}

func TestTriggerVersionOwnershipAndDeletion(t *testing.T) {
	data := newTestSQLiteStore(t)
	item := storedTrigger(t, data)
	if _, err := data.GetTrigger(t.Context(), "another-client", item.ID); !errors.Is(err, trigger.ErrNotFound) {
		t.Fatalf("cross-client get: %v", err)
	}
	if err := data.DeleteTrigger(t.Context(), "another-client", item.ID, item.Version); !errors.Is(err, trigger.ErrConflict) {
		t.Fatalf("cross-client delete: %v", err)
	}
	paused := item
	paused.Enabled, paused.NextFireAt = false, nil
	updated, err := data.SaveTrigger(t.Context(), paused, item.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.SaveTrigger(t.Context(), item, item.Version); !errors.Is(err, trigger.ErrConflict) {
		t.Fatalf("stale update: %v", err)
	}
	if _, err := data.CommitTriggerOccurrence(t.Context(), triggerSubmission(item)); !errors.Is(err, trigger.ErrConflict) {
		t.Fatalf("stale fire: %v", err)
	}
	manual := triggerSubmission(item)
	manual.Trigger = updated
	manual.Occurrence.Source = "manual"
	result, err := data.CommitTriggerOccurrence(t.Context(), manual)
	if err != nil || !result.Admission.Created {
		t.Fatalf("paused manual: %#v, %v", result, err)
	}
	if err := data.DeleteTrigger(t.Context(), updated.ClientID, updated.ID, updated.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := data.GetSession(result.Occurrence.SessionID); err != nil {
		t.Fatalf("deletion removed session: %v", err)
	}
	items, err := data.ListTriggerOccurrences(t.Context(), item.ClientID, "", 50)
	if err != nil || len(items) != 0 {
		t.Fatalf("history after delete: %#v, %v", items, err)
	}
}
