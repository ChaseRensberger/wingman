package models

import (
	"sync"
)

// EventStream is a generic buffered channel for streaming values of type T
// and terminating with a final value of type F.
type EventStream[T any, F any] struct {
	ch     chan T
	final  F
	err    error
	closed bool
	mu     sync.Mutex
}

// NewEventStream creates an EventStream with a buffer of size buf.
func NewEventStream[T any, F any](buf int) *EventStream[T, F] {
	return &EventStream[T, F]{ch: make(chan T, buf)}
}

// Push sends a value into the stream. It panics if called after Close.
func (es *EventStream[T, F]) Push(v T) {
	es.ch <- v
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
