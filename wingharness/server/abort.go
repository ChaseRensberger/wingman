package server

import (
	"context"
	"sync"
)

// abortRegistry tracks in-flight per-session contexts so an abort
// request can cancel them. A single session may have multiple
// concurrent runs (e.g. a streaming request and a non-streaming one
// racing the same client; or two clients of the same session). We
// store a slice of entries and fire cancel on all of them on Abort.
//
// The registry is safe for concurrent use. Callers MUST invoke the
// returned release func when their work completes (defer) so the
// registration is removed; leaving stale entries in the slice would
// cause a future Abort to also fire on already-finished cancels (no
// harm — cancel is idempotent — but the count returned by abort would
// mislead diagnostics and the slice would leak memory in long-lived
// processes).
type abortRegistry struct {
	mu      sync.Mutex
	entries map[string][]*abortEntry
}

// abortEntry is a heap-allocated wrapper so we have a unique pointer
// identity for removal. (Go funcs aren't comparable, so we can't
// directly diff a CancelFunc out of a slice.)
type abortEntry struct {
	cancel context.CancelFunc
}

func newAbortRegistry() *abortRegistry {
	return &abortRegistry{entries: map[string][]*abortEntry{}}
}

// register derives a cancellable child of parent and records it
// against sessionID. Returns the child ctx and a release func that
// removes the registration and cancels the child (idempotent — safe to
// call after Abort already fired). Use as:
//
//	ctx, release := s.aborts.register(sessionID, r.Context())
//	defer release()
func (a *abortRegistry) register(sessionID string, parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	entry := &abortEntry{cancel: cancel}

	a.mu.Lock()
	a.entries[sessionID] = append(a.entries[sessionID], entry)
	a.mu.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			a.mu.Lock()
			list := a.entries[sessionID]
			out := list[:0]
			for _, e := range list {
				if e != entry {
					out = append(out, e)
				}
			}
			if len(out) == 0 {
				delete(a.entries, sessionID)
			} else {
				a.entries[sessionID] = out
			}
			a.mu.Unlock()
			cancel()
		})
	}
	return ctx, release
}

// abort cancels every in-flight context registered for sessionID and
// returns the count cancelled. Safe to call when nothing is registered
// (returns 0).
//
// Cancels run while we hold no lock so a release racing with abort
// can't deadlock on a.mu.
func (a *abortRegistry) abort(sessionID string) int {
	a.mu.Lock()
	list := a.entries[sessionID]
	delete(a.entries, sessionID)
	a.mu.Unlock()

	for _, e := range list {
		e.cancel()
	}
	return len(list)
}
