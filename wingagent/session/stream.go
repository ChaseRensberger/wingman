package session

import (
	"context"
	"fmt"

	"github.com/chaserensberger/wingman/wingagent/loop"
)

// SessionStream is the streaming counterpart to Session.Run. It exposes
// the loop's lifecycle and provider stream parts as a serial sequence
// the caller drains via Next/Event, plus a terminal Result accessible
// after Next returns false.
//
// Concurrency: SessionStream is single-consumer. Behind the scenes, the
// loop runs on a background goroutine; events are forwarded to a
// buffered channel. If the consumer stops calling Next, the goroutine
// blocks on the channel and the loop stalls; cancel ctx to abort.
type SessionStream struct {
	events  chan StreamEvent
	resultC chan streamResult

	current StreamEvent
	result  *Result
	err     error
	done    bool
}

// StreamEvent is the unit of the SessionStream. Type names are stable
// strings suitable for SSE event names. Data carries the per-type
// payload, JSON-encodable by the standard library.
//
// Defined event types:
//
//   - "iteration_start": Data is loop.IterationStartEvent
//   - "iteration_end":   Data is loop.IterationEndEvent
//   - "message":         Data is loop.MessageEvent
//   - "tool_start":      Data is loop.ToolExecutionStartEvent
//   - "tool_end":        Data is loop.ToolExecutionEndEvent
//   - "stream_part":     Data is loop.StreamPartEvent (carries wingmodels.StreamPart)
//   - "error":           Data is loop.ErrorEvent
//
// Consumers that want the loop's typed events simply type-assert on Data.
type StreamEvent struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// streamResult ferries the loop's Result + error across the goroutine
// boundary. Sent exactly once on resultC.
type streamResult struct {
	res *Result
	err error
}

// RunStream is the streaming counterpart to Run. It returns immediately
// after starting the loop on a background goroutine; the caller drains
// events via Next/Event and reads the terminal Result after the channel
// closes.
//
// The session's history is updated by the underlying Run path on the
// background goroutine. Callers reading History() concurrently with an
// in-flight stream will see history snapshots that grow as turns
// complete.
func (s *Session) RunStream(ctx context.Context, message string) (*SessionStream, error) {
	s.mu.RLock()
	if s.model == nil {
		s.mu.RUnlock()
		return nil, ErrNoModel
	}
	s.mu.RUnlock()

	ss := &SessionStream{
		// 256 = comfortable headroom for streaming text deltas in a
		// single turn before backpressuring the loop. Smaller would risk
		// stalling; larger wastes memory on slow consumers.
		events:  make(chan StreamEvent, 256),
		resultC: make(chan streamResult, 1),
	}

	// Forwarding sink: convert each loop.Event into a StreamEvent and
	// push onto ss.events. The loop emits on its own goroutine, so the
	// sink will block when ss.events is full; that's intentional
	// backpressure on slow consumers.
	sink := loop.SinkFunc(func(e loop.Event) {
		ev := toStreamEvent(e)
		select {
		case ss.events <- ev:
		case <-ctx.Done():
			// Drop on cancel rather than block forever.
		}
	})

	go func() {
		defer close(ss.events)
		res, err := s.runWith(ctx, message, sink)
		ss.resultC <- streamResult{res: res, err: err}
	}()

	return ss, nil
}

// toStreamEvent classifies a loop.Event into the public envelope. Adding
// a new loop event variant requires updating this switch; the default
// branch surfaces the raw event under an "unknown" type so logs catch
// the omission.
func toStreamEvent(e loop.Event) StreamEvent {
	switch v := e.(type) {
	case loop.IterationStartEvent:
		return StreamEvent{Type: "iteration_start", Data: v}
	case loop.IterationEndEvent:
		return StreamEvent{Type: "iteration_end", Data: v}
	case loop.MessageEvent:
		return StreamEvent{Type: "message", Data: v}
	case loop.ToolExecutionStartEvent:
		return StreamEvent{Type: "tool_start", Data: v}
	case loop.ToolExecutionEndEvent:
		return StreamEvent{Type: "tool_end", Data: v}
	case loop.StreamPartEvent:
		return StreamEvent{Type: "stream_part", Data: v}
	case loop.ErrorEvent:
		return StreamEvent{Type: "error", Data: map[string]string{"error": fmt.Sprint(v.Err)}}
	default:
		return StreamEvent{Type: "unknown", Data: v}
	}
}

// Next blocks for the next event, returning false when the stream is
// exhausted (loop done or aborted). After Next returns false, callers
// should consult Err and Result.
func (ss *SessionStream) Next() bool {
	if ss.done {
		return false
	}
	ev, ok := <-ss.events
	if !ok {
		// Channel closed: drain the terminal result.
		ss.done = true
		r := <-ss.resultC
		ss.result = r.res
		ss.err = r.err
		return false
	}
	ss.current = ev
	return true
}

// Event returns the most recent event Next surfaced.
func (ss *SessionStream) Event() StreamEvent { return ss.current }

// Err returns the loop error, if any, after Next returns false.
func (ss *SessionStream) Err() error { return ss.err }

// Result returns the terminal Result after Next returns false. It is
// always non-nil so callers can persist partial state on errors. The
// Result mirrors what Session.Run would have returned synchronously,
// minus any tool calls whose ToolExecutionEndEvent was dropped due to
// ctx cancellation.
func (ss *SessionStream) Result() *Result {
	// Defensive: callers occasionally call Result before draining; in
	// that case we have no result yet. Returning nil there leaks the
	// sentinel that they must drain first.
	if ss.result == nil {
		return &Result{}
	}
	return ss.result
}
