package run

import "context"

// ToolLimiter bounds tool executions across runs that share it.
type ToolLimiter struct{ slots chan struct{} }

// NewToolLimiter creates a shared limit; non-positive limits use sixteen slots.
func NewToolLimiter(limit int) *ToolLimiter {
	if limit <= 0 {
		limit = 16
	}
	return &ToolLimiter{slots: make(chan struct{}, limit)}
}

func (l *ToolLimiter) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case l.slots <- struct{}{}:
		if err := ctx.Err(); err != nil {
			l.release()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *ToolLimiter) release() { <-l.slots }
