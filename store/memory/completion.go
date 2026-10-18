package memory

import (
	"context"

	"github.com/chaserensberger/wingman/store"
)

// UpdateSessionRunCompletion commits a candidate decision and its durable event.
func (s *Store) UpdateSessionRunCompletion(ctx context.Context, update store.CompletionUpdate) (store.SessionRunTransition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return store.SessionRunTransition{}, err
	}
	current, ok := s.runs[update.RunID]
	if !ok {
		return store.SessionRunTransition{}, store.ErrSessionRunNotFound
	}
	run, changed, err := store.ApplyCompletionUpdate(copySessionRun(current), update)
	if err != nil || !changed {
		return store.SessionRunTransition{Run: run}, err
	}
	aggregate, err := store.NewSessionCompletionEvent(run)
	if err != nil {
		return store.SessionRunTransition{}, err
	}
	session := s.sessions[run.SessionID]
	aggregate.Version = session.AggregateVersion + 1
	projected, err := store.ApplySessionEvent(session, aggregate)
	if err != nil {
		return store.SessionRunTransition{}, err
	}
	event, err := s.appendRunEventLocked(&run, store.EventSessionRunCompletion, map[string]any{"completion": run.Completions[len(run.Completions)-1]}, run.UpdatedAt)
	if err != nil {
		return store.SessionRunTransition{}, err
	}
	s.globalSeq++
	aggregate.GlobalSequence = s.globalSeq
	s.aggregates[aggregate.Aggregate] = append(s.aggregates[aggregate.Aggregate], copyAggregateEvent(aggregate))
	s.sessions[run.SessionID] = copySession(projected)
	*current = copySessionRun(&run)
	return store.SessionRunTransition{Run: run, Event: event, Changed: true}, nil
}
