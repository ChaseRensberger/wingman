package plugin

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/chaserensberger/wingman/plugin/protocol"
)

// Subscription receives client-owned public and plugin events until cancellation or overflow.
type Subscription struct {
	host    *Host
	ctx     context.Context
	id      string
	once    sync.Once
	cleanup context.Context
	cancel  context.CancelFunc
	stop    func() bool
	err     error
}

// SubscribeEvents opens a filtered subscription; use a Background context to outlive an invocation.
func (h *Host) SubscribeEvents(ctx context.Context, types ...string) (*Subscription, error) {
	background, err := h.Background(ctx)
	if err != nil {
		return nil, err
	}
	cleanup := func() {
		ctx, cancel := context.WithTimeout(background, 5*time.Second)
		defer cancel()
		_ = h.Call(ctx, "scope.release", nil, nil)
	}
	owned, cancel := context.WithCancel(background)
	stop := context.AfterFunc(ctx, cancel)
	var id string
	if err := h.Call(owned, "events.subscribe", map[string]any{"types": types}, &id); err != nil {
		stop()
		cancel()
		cleanup()
		return nil, err
	}
	s := &Subscription{host: h, ctx: owned, cleanup: background, cancel: cancel, stop: stop, id: id}
	context.AfterFunc(owned, func() { _ = s.Close() })
	return s, nil
}

// Next waits for the next batch; an empty batch is a long-poll timeout.
func (s *Subscription) Next() ([]protocol.Event, error) {
	var result []protocol.Event
	err := s.host.Call(s.ctx, "events.next", map[string]string{"id": s.id}, &result)
	return result, err
}

// Close releases the server subscription.
func (s *Subscription) Close() error {
	s.once.Do(func() {
		s.stop()
		s.cancel()
		ctx, cancel := context.WithTimeout(s.cleanup, 5*time.Second)
		defer cancel()
		s.err = s.host.Call(ctx, "events.close", map[string]string{"id": s.id}, nil)
		_ = s.host.Call(ctx, "scope.release", nil, nil)
	})
	return s.err
}

// PublishEvent emits an event under this plugin's namespace to the owning client.
func (h *Host) PublishEvent(ctx context.Context, name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return h.Call(ctx, "events.publish", map[string]any{"type": name, "data": json.RawMessage(data)}, nil)
}
