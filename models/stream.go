package models

import (
	"errors"
	"iter"
	"sync"
)

// EventStream is a one-producer, one-consumer event channel with a single
// final-result slot. Adapted from pi-mono's TypeScript EventStream<T,R>
	// (bb/pi-mono/packages/ai/src/scripts/event-stream.ts), translated to Go using
// a buffered channel for events and a sync.Once for the final result.
//
// Usage on the producer side:
//
//	stream := NewEventStream[StreamPart, *Message](64)
//	go func() {
//	    defer stream.Close(finalMsg, nil) // always close exactly once
//	    stream.Push(StreamStartPart{})
//	    // ...
//	}()
//	return stream, nil
//
// Usage on the consumer side:
//
//	for part := range stream.Iter() {
//	    // handle part
//	}
//	msg, err := stream.Final()
//
// Concurrency:
//   - Push and Close are safe from a single producer goroutine. Calling Push
//     after Close panics on a closed channel; producers must structure code
//     so Close is the last operation.
//   - Iter and Final are safe from a single consumer goroutine.
//   - Final blocks until Close has been called, even if the consumer hasn't
//     drained Iter yet.
type EventStream[E any, R any] struct {
	events chan E

	closeOnce sync.Once
	done      chan struct{}
	result    R
	err       error
}

// NewEventStream constructs a stream with the given buffered event capacity.
// Cap should be sized to absorb a typical burst of stream parts without
// blocking the producer; 64 is a reasonable default for LLM streams.
func NewEventStream[E any, R any](cap int) *EventStream[E, R] {
	if cap < 0 {
		cap = 0
	}
	return &EventStream[E, R]{
		events: make(chan E, cap),
		done:   make(chan struct{}),
	}
}

// Push delivers an event to the stream. Blocks if the buffer is full and the
// consumer is not draining; this is the intended back-pressure behavior.
//
// Push after Close is a programmer error and panics (sending on a closed
// channel). Producers must structure code so Close is the last call.
func (s *EventStream[E, R]) Push(event E) {
	s.events <- event
}

// Close terminates the stream with a final result and optional error. Safe to
// call multiple times; only the first call is recorded. Closing the events
// channel signals the iterator to stop.
//
// Pass a zero R if there is no meaningful result (e.g. on early error). Pass
// nil err on success.
func (s *EventStream[E, R]) Close(result R, err error) {
	s.closeOnce.Do(func() {
		s.result = result
		s.err = err
		close(s.events)
		close(s.done)
	})
}

// Iter returns an iter.Seq over events. Ranging continues until Close is
// called and the buffer is drained.
//
// Single-consumer: only one goroutine should range over Iter. Multiple
// consumers would race on channel receives and observe arbitrary partitions
// of the event stream.
func (s *EventStream[E, R]) Iter() iter.Seq[E] {
	return func(yield func(E) bool) {
		for ev := range s.events {
			if !yield(ev) {
				return
			}
		}
	}
}

// Final blocks until Close has been called, then returns the final result and
// error recorded by Close. Safe to call from any goroutine, multiple times.
//
// Final does not drain the events channel; callers that haven't ranged over
// Iter will not observe individual events but Final still resolves once the
// producer has called Close. In practice consumers either:
//   - Range Iter, then call Final to get the assembled result; or
//   - Skip Iter entirely and only care about the final outcome (e.g.
//     models.Run does this for synchronous calls).
func (s *EventStream[E, R]) Final() (R, error) {
	<-s.done
	return s.result, s.err
}

// ErrStreamNotClosed is reserved for future use by a non-blocking accessor.
// Currently unused; Final blocks instead of returning this. Kept exported
// so callers can refer to it without re-declaration.
var ErrStreamNotClosed = errors.New("stream not yet closed")
