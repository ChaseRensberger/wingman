package store

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func TestSQLiteCreateSessionCommitsEventAndProjection(t *testing.T) {
	data := newTestSQLiteStore(t)
	session := &Session{
		ID:      "ses_created",
		Title:   "Created from an event",
		WorkDir: "/tmp/wingman",
	}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	if session.AggregateVersion != 1 {
		t.Fatalf("aggregate version = %d, want 1", session.AggregateVersion)
	}

	events, err := data.ListAggregateEvents(context.Background(), AggregateRef{Type: AggregateSession, ID: session.ID}, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	event := events[0]
	if event.Version != 1 || event.Type != EventSessionCreated || event.SchemaVersion != 1 {
		t.Fatalf("event = %#v", event)
	}
	if event.GlobalSequence == 0 {
		t.Fatal("global sequence was not assigned")
	}

	projected, err := ProjectSession(events)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := data.GetSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projected, stored) {
		t.Fatalf("replayed projection = %#v, stored = %#v", projected, stored)
	}
}

func TestSQLiteCreateSessionRollsBackEventWhenProjectionFails(t *testing.T) {
	data := newTestSQLiteStore(t)
	if _, err := data.db.Exec(`
		INSERT INTO sessions (id, title, created_at, updated_at, aggregate_version)
		VALUES ('ses_existing', 'legacy row', ?, ?, 0)
	`, Now(), Now()); err != nil {
		t.Fatal(err)
	}

	err := data.CreateSession(&Session{ID: "ses_existing", Title: "conflict"})
	if err == nil {
		t.Fatal("CreateSession succeeded, want projection error")
	}
	events, listErr := data.ListAggregateEvents(context.Background(), AggregateRef{Type: AggregateSession, ID: "ses_existing"}, 0, 100)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(events) != 0 {
		t.Fatalf("events = %d, want rollback to leave 0", len(events))
	}
	stored, getErr := data.GetSession("ses_existing")
	if getErr != nil {
		t.Fatal(getErr)
	}
	if stored.Title != "legacy row" || stored.AggregateVersion != 0 {
		t.Fatalf("stored session = %#v", stored)
	}
}

func TestSQLiteCreateSessionSerializesConcurrentCreation(t *testing.T) {
	data := newTestSQLiteStore(t)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- data.CreateSession(&Session{ID: "ses_concurrent"})
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	var successes, conflicts int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrAggregateVersionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes = %d, conflicts = %d; want 1 each", successes, conflicts)
	}
	events, err := data.ListAggregateEvents(context.Background(), AggregateRef{Type: AggregateSession, ID: "ses_concurrent"}, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Version != 1 {
		t.Fatalf("events = %#v", events)
	}
}

func TestSQLiteCreateSessionSerializesAcrossStoreHandles(t *testing.T) {
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

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, data := range []*SQLiteStore{first, second} {
		wg.Add(1)
		go func(data *SQLiteStore) {
			defer wg.Done()
			<-start
			errs <- data.CreateSession(&Session{ID: "ses_multi_handle"})
		}(data)
	}
	close(start)
	wg.Wait()
	close(errs)

	var successes, conflicts int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrAggregateVersionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes = %d, conflicts = %d; want 1 each", successes, conflicts)
	}
}

func TestProjectSessionIsDeterministic(t *testing.T) {
	session := Session{
		ID:        "ses_replay",
		Title:     "Replay me",
		CreatedAt: "2026-07-30T12:00:00Z",
		UpdatedAt: "2026-07-30T12:00:00Z",
	}
	event, err := NewSessionCreatedEvent(session)
	if err != nil {
		t.Fatal(err)
	}
	event.Version = 1
	first, err := ProjectSession([]AggregateEvent{event})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ProjectSession([]AggregateEvent{event})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("first = %#v, second = %#v", first, second)
	}
}

func TestProjectSessionRejectsInvalidStream(t *testing.T) {
	_, err := ProjectSession([]AggregateEvent{{
		Aggregate:     AggregateRef{Type: AggregateSession, ID: "ses_invalid"},
		Version:       2,
		Type:          EventSessionCreated,
		SchemaVersion: 1,
		Data:          []byte(`{"id":"ses_invalid"}`),
	}})
	if err == nil {
		t.Fatal("ProjectSession succeeded with a gapped stream")
	}
}

func newTestSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	data, err := NewSQLiteStore(filepath.Join(t.TempDir(), "wingman.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	return data
}
