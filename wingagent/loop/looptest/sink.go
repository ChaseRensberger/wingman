package looptest

import (
	"sync"

	"github.com/chaserensberger/wingman/wingagent/loop"
)

// RecordingSink implements loop.Sink and stores all events for later
// inspection.
type RecordingSink struct {
	mu     sync.Mutex
	events []loop.Event
}

// NewRecordingSink constructs a new RecordingSink.
func NewRecordingSink() *RecordingSink { return &RecordingSink{} }

// OnEvent implements loop.Sink.
func (s *RecordingSink) OnEvent(e loop.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
}

// Events returns a copy of all recorded events.
func (s *RecordingSink) Events() []loop.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]loop.Event, len(s.events))
	copy(out, s.events)
	return out
}

// IterationStarts returns every IterationStartEvent.
func (s *RecordingSink) IterationStarts() []loop.IterationStartEvent { return filter[loop.IterationStartEvent](s) }

// IterationEnds returns every IterationEndEvent.
func (s *RecordingSink) IterationEnds() []loop.IterationEndEvent { return filter[loop.IterationEndEvent](s) }

// Messages returns every MessageEvent.
func (s *RecordingSink) Messages() []loop.MessageEvent { return filter[loop.MessageEvent](s) }

// ToolStarts returns every ToolExecutionStartEvent.
func (s *RecordingSink) ToolStarts() []loop.ToolExecutionStartEvent { return filter[loop.ToolExecutionStartEvent](s) }

// ToolEnds returns every ToolExecutionEndEvent.
func (s *RecordingSink) ToolEnds() []loop.ToolExecutionEndEvent { return filter[loop.ToolExecutionEndEvent](s) }

// StreamParts returns every StreamPartEvent.
func (s *RecordingSink) StreamParts() []loop.StreamPartEvent { return filter[loop.StreamPartEvent](s) }

// Errors returns every ErrorEvent.
func (s *RecordingSink) Errors() []loop.ErrorEvent { return filter[loop.ErrorEvent](s) }

// ContextTransforms returns every ContextTransformedEvent.
func (s *RecordingSink) ContextTransforms() []loop.ContextTransformedEvent { return filter[loop.ContextTransformedEvent](s) }

func filter[T loop.Event](s *RecordingSink) []T {
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
