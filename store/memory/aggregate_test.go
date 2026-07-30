package memory

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/chaserensberger/wingman/store"
)

func TestCreateSessionCommitsEventAndProjection(t *testing.T) {
	data := NewStore()
	session := &store.Session{ID: "ses_memory", Title: "Memory"}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	events, err := data.ListAggregateEvents(context.Background(), store.AggregateRef{Type: store.AggregateSession, ID: session.ID}, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := store.ProjectSession(events)
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

func TestCreateSessionRejectsDuplicateAggregate(t *testing.T) {
	data := NewStore()
	if err := data.CreateSession(&store.Session{ID: "ses_duplicate"}); err != nil {
		t.Fatal(err)
	}
	err := data.CreateSession(&store.Session{ID: "ses_duplicate"})
	if !errors.Is(err, store.ErrAggregateVersionConflict) {
		t.Fatalf("error = %v, want aggregate version conflict", err)
	}
	events, listErr := data.ListAggregateEvents(context.Background(), store.AggregateRef{Type: store.AggregateSession, ID: "ses_duplicate"}, 0, 100)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}

func TestSessionMetadataEventsUpdateProjectionAndReplay(t *testing.T) {
	data := NewStore()
	session := &store.Session{ID: "ses_update", Title: "Before", WorkDir: "/before"}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	renamed, err := data.RenameSession(context.Background(), session.ID, "After", 1)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := data.MoveSession(context.Background(), session.ID, "/after", "", renamed.AggregateVersion)
	if err != nil {
		t.Fatal(err)
	}
	if moved.AggregateVersion != 3 || moved.Title != "After" || moved.WorkDir != "/after" {
		t.Fatalf("moved session = %#v", moved)
	}
	events, err := data.ListAggregateEvents(context.Background(), store.AggregateRef{Type: store.AggregateSession, ID: session.ID}, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := store.ProjectSession(events)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projected, moved) {
		t.Fatalf("replayed projection = %#v, stored = %#v", projected, moved)
	}
}

func TestSessionMetadataNoOpAndConflict(t *testing.T) {
	data := NewStore()
	session := &store.Session{ID: "ses_conflict", Title: "Before"}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	unchanged, err := data.RenameSession(context.Background(), session.ID, "Before", 1)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.AggregateVersion != 1 {
		t.Fatalf("no-op version = %d, want 1", unchanged.AggregateVersion)
	}
	if _, err := data.RenameSession(context.Background(), session.ID, "After", 1); err != nil {
		t.Fatal(err)
	}
	_, err = data.MoveSession(context.Background(), session.ID, "/stale", "", 1)
	if !errors.Is(err, store.ErrAggregateVersionConflict) {
		t.Fatalf("error = %v, want aggregate version conflict", err)
	}
	events, listErr := data.ListAggregateEvents(context.Background(), store.AggregateRef{Type: store.AggregateSession, ID: session.ID}, 0, 100)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
}
