package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/chaserensberger/wingman/store"
)

func TestSessionEventWatermark(t *testing.T) {
	data := NewStore()
	ctx := context.Background()
	if err := data.CreateSession(&store.Session{ID: "ses_watermark"}); err != nil {
		t.Fatal(err)
	}
	if watermark, err := data.SessionEventWatermark(ctx, "ses_watermark"); err != nil || watermark != 0 {
		t.Fatalf("empty watermark = %d, %v; want 0, nil", watermark, err)
	}
	if _, err := data.SessionEventWatermark(ctx, "ses_missing"); !errors.Is(err, store.ErrSessionNotFound) {
		t.Fatalf("missing watermark error = %v, want %v", err, store.ErrSessionNotFound)
	}
	if _, err := data.AppendSessionEvent(ctx, store.SessionEvent{SessionID: "ses_watermark", Type: "test"}); err != nil {
		t.Fatal(err)
	}
	if watermark, err := data.SessionEventWatermark(ctx, "ses_watermark"); err != nil || watermark != 1 {
		t.Fatalf("watermark = %d, %v; want 1, nil", watermark, err)
	}
}
