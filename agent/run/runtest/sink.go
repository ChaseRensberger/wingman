package runtest

import (
	"sync"

	"github.com/chaserensberger/wingman/agent/run"
)

// RecordingSink implements run.Sink and stores all events for later
// inspection.
type RecordingSink struct {
	mu     sync.Mutex
	events []run.Event
}

// NewRecordingSink constructs a new RecordingSink.
func NewRecordingSink() *RecordingSink { return &RecordingSink{} }

// OnEvent implements run.Sink.
func (s *RecordingSink) OnEvent(e run.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
}

// Events returns a copy of all recorded events.
func (s *RecordingSink) Events() []run.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]run.Event, len(s.events))
	copy(out, s.events)
	return out
}

// IterationStarts returns every IterationStartEvent.
func (s *RecordingSink) IterationStarts() []run.IterationStartEvent {
	return filter[run.IterationStartEvent](s)
}

// IterationEnds returns every IterationEndEvent.
func (s *RecordingSink) IterationEnds() []run.IterationEndEvent {
	return filter[run.IterationEndEvent](s)
}

// Messages returns every MessageEvent.
func (s *RecordingSink) Messages() []run.MessageEvent { return filter[run.MessageEvent](s) }

// ToolStarts returns every ToolExecutionStartEvent.
func (s *RecordingSink) ToolStarts() []run.ToolExecutionStartEvent {
	return filter[run.ToolExecutionStartEvent](s)
}

// ToolEnds returns every ToolExecutionEndEvent.
func (s *RecordingSink) ToolEnds() []run.ToolExecutionEndEvent {
	return filter[run.ToolExecutionEndEvent](s)
}

// StreamParts returns every StreamPartEvent.
func (s *RecordingSink) StreamParts() []run.StreamPartEvent { return filter[run.StreamPartEvent](s) }

// Errors returns every ErrorEvent.
func (s *RecordingSink) Errors() []run.ErrorEvent { return filter[run.ErrorEvent](s) }

// ContextTransforms returns every ContextTransformedEvent.
func (s *RecordingSink) ContextTransforms() []run.ContextTransformedEvent {
	return filter[run.ContextTransformedEvent](s)
}

// StructuredOutputs returns every StructuredOutputEvent.
func (s *RecordingSink) StructuredOutputs() []run.StructuredOutputEvent {
	return filter[run.StructuredOutputEvent](s)
}

func filter[T run.Event](s *RecordingSink) []T {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []T
	for _, e := range s.events {
		if v, ok := e.(T); ok {
			out = append(out, v)
		}
	}
	return out
}
