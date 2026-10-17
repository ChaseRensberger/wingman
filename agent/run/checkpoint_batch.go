package run

import (
	"context"
	"time"

	"github.com/chaserensberger/wingman/models"
)

const checkpointBatchInterval = 100 * time.Millisecond

type checkpointBatch struct {
	events []StreamPartEvent
	bytes  int
	timer  *time.Timer
}

func (b *checkpointBatch) ready() <-chan time.Time {
	if b.timer == nil {
		return nil
	}
	return b.timer.C
}

func (b *checkpointBatch) stop() {
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
}

func (b *checkpointBatch) add(event StreamPartEvent, size int) {
	if len(b.events) == 0 {
		b.timer = time.NewTimer(checkpointBatchInterval)
	}
	b.events = append(b.events, event)
	b.bytes += size
}

func (b *checkpointBatch) full() bool { return len(b.events) >= 64 || b.bytes >= 16*1024 }

func (b *checkpointBatch) flush(ctx context.Context, r *runner, step int, message *models.Message) error {
	if len(b.events) == 0 {
		return nil
	}
	if err := r.checkpoint(ctx, step, message); err != nil {
		return err
	}
	// Live deltas must not get ahead of the snapshot used for reconnect and recovery.
	for _, event := range b.events {
		r.emit(event)
	}
	b.stop()
	b.events, b.bytes = nil, 0
	return nil
}

func streamDeltaBytes(part models.StreamPart) (int, bool) {
	switch p := part.(type) {
	case models.TextDeltaPart:
		return len(p.Delta), true
	case models.ReasoningDeltaPart:
		return len(p.Delta) + len(p.Encrypted), true
	case models.ToolInputDeltaPart:
		return len(p.Delta), true
	default:
		return 0, false
	}
}
