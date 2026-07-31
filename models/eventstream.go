package models

import (
	"context"
	"sync"
)

// EventStream is a generic buffered channel for streaming values of type T
// and terminating with a final value of type F.
type EventStream[T any, F any] struct {
	ch     chan T
	final  F
	err    error
	closed bool
	done   <-chan struct{}
	mu     sync.Mutex
}

// BindContext makes Push return without blocking after ctx is cancelled.
// It must be called before producers start pushing values.
func (es *EventStream[T, F]) BindContext(ctx context.Context) {
	es.mu.Lock()
	defer es.mu.Unlock()
	es.done = ctx.Done()
}

// NewEventStream creates an EventStream with a buffer of size buf.
func NewEventStream[T any, F any](buf int) *EventStream[T, F] {
	return &EventStream[T, F]{ch: make(chan T, buf)}
}

// Push sends a value into the stream. It panics if called after Close.
func (es *EventStream[T, F]) Push(v T) {
	es.mu.Lock()
	done := es.done
	es.mu.Unlock()
	if done == nil {
		es.ch <- v
		return
	}
	select {
	case es.ch <- v:
	case <-done:
	}
}

// Close signals the end of the stream, setting the final value and error.
func (es *EventStream[T, F]) Close(final F, err error) {
	es.mu.Lock()
	defer es.mu.Unlock()
	if es.closed {
		return
	}
	es.closed = true
	es.final = final
	es.err = err
	close(es.ch)
}

// Iter returns the receive-only channel for draining stream values.
func (es *EventStream[T, F]) Iter() <-chan T {
	return es.ch
}

// Final returns the terminal value and any error after Iter is exhausted.
func (es *EventStream[T, F]) Final() (F, error) {
	es.mu.Lock()
	defer es.mu.Unlock()
	return es.final, es.err
}

// Drain consumes all stream values and returns the terminal value and error.
func (es *EventStream[T, F]) Drain() (F, error) {
	for range es.Iter() {
	}
	return es.Final()
}
