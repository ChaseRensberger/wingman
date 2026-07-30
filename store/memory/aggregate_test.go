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

func TestUpdateSessionPreservesAggregateVersion(t *testing.T) {
	data := NewStore()
	session := &store.Session{ID: "ses_update", Title: "Before"}
	if err := data.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	updated := &store.Session{ID: session.ID, Title: "After"}
	if err := data.UpdateSession(updated); err != nil {
		t.Fatal(err)
	}
	stored, err := data.GetSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AggregateVersion != 1 {
		t.Fatalf("aggregate version = %d, want 1", stored.AggregateVersion)
	}
}
