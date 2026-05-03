package session

import (
	"context"
	"fmt"

	"github.com/chaserensberger/wingman/wingharness/loop"
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
// payload, JSON-encodable by the standard library. Version is the
// envelope schema version (currently EnvelopeVersion = 1); consumers
// should refuse to interpret payloads with a Version they don't
// recognize. The version describes the *envelope*, not the inner Data
// shape — Data shapes are governed by their respective loop event
// types' Go struct tags and may evolve additively (new optional fields)
// without bumping Version.
//
// Defined event types:
//
//   - "iteration_start":     Data is loop.IterationStartEvent
//   - "iteration_end":       Data is loop.IterationEndEvent
//   - "message":             Data is loop.MessageEvent
//   - "tool_start":          Data is loop.ToolExecutionStartEvent
//   - "tool_end":            Data is loop.ToolExecutionEndEvent
//   - "stream_part":         Data is loop.StreamPartEvent (carries wingmodels.StreamPart)
//   - "compaction":          Data is loop.ContextTransformedEvent (head Part type "compaction_marker")
//   - "context_transformed": Data is loop.ContextTransformedEvent (other transforms)
//   - "error":               Data is loop.ErrorEvent
//
// Consumers that want the loop's typed events simply type-assert on Data.
type StreamEvent struct {
	Type    string `json:"type"`
	Version int    `json:"version"`
	Data    any    `json:"data"`
}

// EnvelopeVersion is the current StreamEvent envelope schema version.
// Bump only on breaking changes to the envelope itself (Type/Version/Data
// shape, *not* changes to the Data payload's inner fields).
const EnvelopeVersion = 1

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

// toStreamEvent classifies a loop.Event into the public envelope.
// Adding a new loop event variant requires updating classify; the
// default branch surfaces the raw event under an "unknown" type so logs
// catch the omission. Version is stamped centrally so call sites stay
// uniform — never construct a StreamEvent without using this.
func toStreamEvent(e loop.Event) StreamEvent {
	t, data := classify(e)
	return StreamEvent{Type: t, Version: EnvelopeVersion, Data: data}
}

func classify(e loop.Event) (string, any) {
	switch v := e.(type) {
	case loop.IterationStartEvent:
		return "iteration_start", v
	case loop.IterationEndEvent:
		return "iteration_end", v
	case loop.MessageEvent:
		return "message", v
	case loop.ToolExecutionStartEvent:
		return "tool_start", v
	case loop.ToolExecutionEndEvent:
		return "tool_end", v
	case loop.StreamPartEvent:
		return "stream_part", v
	case loop.ContextTransformedEvent:
		// Discriminate by inspecting the head message's first part
		// type discriminator. Plugins that wish to surface their own
		// SSE event type for a context transform install a Part whose
		// Type() string the wire layer can recognize. We hardcode one
		// well-known name ("compaction_marker") so the canonical
		// compaction plugin gets a distinct UI affordance; other
		// transforms (redaction, injection, …) ride the generic event.
		// Loop and core remain ignorant of plugin Go types — only the
		// string discriminator is consulted.
		if v.Head != nil {
			for _, p := range v.Head.Content {
				if p.Type() == "compaction_marker" {
					return "compaction", v
				}
			}
		}
		return "context_transformed", v
	case loop.ErrorEvent:
		return "error", map[string]string{"error": fmt.Sprint(v.Err)}
	default:
		return "unknown", v
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
