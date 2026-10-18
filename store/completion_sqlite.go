package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// UpdateSessionRunCompletion commits a candidate decision and publishes its durable event.
func (s *SQLiteStore) UpdateSessionRunCompletion(ctx context.Context, update CompletionUpdate) (SessionRunTransition, error) {
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return SessionRunTransition{}, err
	}
	defer tx.Rollback()
	run, err := scanSessionRun(tx.QueryRowContext(ctx, `SELECT `+sessionRunColumns+` FROM session_runs WHERE id = ? AND session_id = ?`, update.RunID, update.SessionID))
	if errors.Is(err, sql.ErrNoRows) {
		return SessionRunTransition{}, ErrSessionRunNotFound
	}
	if err != nil {
		return SessionRunTransition{}, err
	}
	run, changed, err := ApplyCompletionUpdate(run, update)
	if err != nil || !changed {
		return SessionRunTransition{Run: run}, err
	}
	encoded, err := json.Marshal(run.Completions)
	if err != nil {
		return SessionRunTransition{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE session_runs SET status = ?, completions_json = ?, updated_at = ? WHERE id = ?`, run.Status, string(encoded), formatTime(run.UpdatedAt), run.ID); err != nil {
		return SessionRunTransition{}, err
	}
	session, err := getSessionTx(ctx, tx, run.SessionID)
	if err != nil {
		return SessionRunTransition{}, err
	}
	aggregate, err := NewSessionCompletionEvent(run)
	if err != nil {
		return SessionRunTransition{}, err
	}
	aggregate, err = appendAggregateEventTx(ctx, tx, aggregate, session.AggregateVersion)
	if err != nil {
		return SessionRunTransition{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET aggregate_version = ? WHERE id = ?`, aggregate.Version, run.SessionID); err != nil {
		return SessionRunTransition{}, err
	}
	event, err := appendRunEventTx(ctx, tx, run, EventSessionRunCompletion, map[string]any{"completion": run.Completions[len(run.Completions)-1]}, run.UpdatedAt)
	if err != nil {
		return SessionRunTransition{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SessionRunTransition{}, err
	}
	return SessionRunTransition{Run: run, Event: event, Changed: true}, nil
}
