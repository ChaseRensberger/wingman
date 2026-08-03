package store

import (
	"context"
	"errors"
	"testing"
)

func TestSQLiteSessionEventWatermark(t *testing.T) {
	data := newTestSQLiteStore(t)
	ctx := context.Background()
	if err := data.CreateSession(&Session{ID: "ses_watermark"}); err != nil {
		t.Fatal(err)
	}
	if watermark, err := data.SessionEventWatermark(ctx, "ses_watermark"); err != nil || watermark != 0 {
		t.Fatalf("empty watermark = %d, %v; want 0, nil", watermark, err)
	}
	if _, err := data.SessionEventWatermark(ctx, "ses_missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("missing watermark error = %v, want %v", err, ErrSessionNotFound)
	}
	event, err := data.AppendSessionEvent(ctx, SessionEvent{SessionID: "ses_watermark", Type: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if event.SchemaVersion != 1 {
		t.Fatalf("appended schema version = %d, want 1", event.SchemaVersion)
	}
	events, err := data.ListSessionEvents(ctx, "ses_watermark", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].SchemaVersion != 1 {
		t.Fatalf("replayed events = %#v", events)
	}
	if watermark, err := data.SessionEventWatermark(ctx, "ses_watermark"); err != nil || watermark != 1 {
		t.Fatalf("watermark = %d, %v; want 1, nil", watermark, err)
	}
}
