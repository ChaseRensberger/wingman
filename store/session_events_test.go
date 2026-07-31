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
	if _, err := data.AppendSessionEvent(ctx, SessionEvent{SessionID: "ses_watermark", Type: "test"}); err != nil {
		t.Fatal(err)
	}
	if watermark, err := data.SessionEventWatermark(ctx, "ses_watermark"); err != nil || watermark != 1 {
		t.Fatalf("watermark = %d, %v; want 1, nil", watermark, err)
	}
}
