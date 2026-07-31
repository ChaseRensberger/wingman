package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/chaserensberger/wingman/store"
)

func TestSaveMessageRevisioned(t *testing.T) {
	data := NewStore()
	if err := data.CreateSession(&store.Session{ID: "ses_messages"}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	initial := store.StoredMessage{ID: "msg_one", SessionID: "ses_messages", Idx: 1, Role: "assistant", Parts: []store.StoredPart{{ID: "part_one", MessageID: "msg_one", Sequence: 1, Kind: "text", PayloadJSON: []byte(`{"text":"one"}`)}}}
	if err := data.SaveMessage(ctx, initial); err != nil {
		t.Fatal(err)
	}
	if err := data.SaveMessage(ctx, initial); err != nil {
		t.Fatalf("exact retry: %v", err)
	}
	conflict := initial
	conflict.Parts[0].PayloadJSON = []byte(`{"text":"changed"}`)
	if err := data.SaveMessage(ctx, conflict); !errors.Is(err, store.ErrMessageRevisionConflict) {
		t.Fatalf("conflict = %v", err)
	}
	stale := initial
	stale.Revision = 0
	updated := initial
	updated.Revision = 2
	updated.Parts = []store.StoredPart{{ID: "part_two", MessageID: "msg_one", Sequence: 1, Kind: "text", PayloadJSON: []byte(`{"text":"two"}`)}}
	if err := data.SaveMessage(ctx, updated); err != nil {
		t.Fatal(err)
	}
	if err := data.SaveMessage(ctx, stale); !errors.Is(err, store.ErrMessageRevisionStale) {
		t.Fatalf("stale = %v", err)
	}
	messages, err := data.ListMessages(ctx, "ses_messages")
	if err != nil || len(messages) != 1 || messages[0].Revision != 2 || len(messages[0].Parts) != 1 || messages[0].Parts[0].ID != "part_two" {
		t.Fatalf("messages=%#v err=%v", messages, err)
	}
}

func TestSaveMessageRejectsInvalidOwnershipAndIndex(t *testing.T) {
	data := NewStore()
	ctx := context.Background()
	message := store.StoredMessage{ID: "msg_one", SessionID: "ses_messages", Idx: 1, Role: "user", Parts: []store.StoredPart{{ID: "part_one", MessageID: "msg_one", Kind: "text", PayloadJSON: []byte(`{}`)}}}
	if err := data.SaveMessage(ctx, message); !errors.Is(err, store.ErrSessionNotFound) {
		t.Fatalf("missing parent = %v", err)
	}
	if err := data.CreateSession(&store.Session{ID: "ses_messages"}); err != nil {
		t.Fatal(err)
	}
	if err := data.SaveMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	if err := data.SaveMessage(ctx, store.StoredMessage{ID: "msg_two", SessionID: "ses_messages", Idx: 1, Role: "user"}); err == nil {
		t.Fatal("duplicate index succeeded")
	}
	if err := data.SaveMessage(ctx, store.StoredMessage{ID: "msg_two", SessionID: "ses_messages", Idx: 2, Role: "user", Parts: []store.StoredPart{{ID: "part_one", MessageID: "msg_two", Kind: "text", PayloadJSON: []byte(`{}`)}}}); err == nil {
		t.Fatal("part ownership conflict succeeded")
	}
}
