package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/chaserensberger/wingman/question"
)

// CreateQuestion saves a request once, including when a tool is resumed.
func (s *SQLiteStore) CreateQuestion(ctx context.Context, q question.Question) (QuestionTransition, error) {
	if err := q.Validate(); err != nil {
		return QuestionTransition{}, err
	}
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return QuestionTransition{}, err
	}
	defer tx.Rollback()
	session, err := getSessionTx(ctx, tx, q.SessionID)
	if err != nil {
		return QuestionTransition{}, err
	}
	q.ClientID = session.ClientID
	if q.ID == "" {
		q.ID = NewID(PrefixQuestion)
	}
	var data string
	err = tx.QueryRowContext(ctx, `SELECT payload_json FROM questions WHERE id = ? OR (tool_use_id = ? AND tool_use_id IS NOT NULL)`, q.ID, nullableString(q.ToolUseID)).Scan(&data)
	if err == nil {
		var existing question.Question
		if err := json.Unmarshal([]byte(data), &existing); err != nil {
			return QuestionTransition{}, err
		}
		if !question.SameRequest(existing, q) {
			return QuestionTransition{}, question.ErrConflict
		}
		return QuestionTransition{Question: existing}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return QuestionTransition{}, err
	}
	if q.RunID != "" {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM session_runs WHERE id = ? AND session_id = ?`, q.RunID, q.SessionID).Scan(&status); err != nil {
			return QuestionTransition{}, ErrSessionRunNotFound
		}
		if status != SessionRunStatusRunning {
			return QuestionTransition{}, question.ErrConflict
		}
	}
	if err := validatePermissionRequestOwnershipTx(ctx, tx, PermissionRequest{SessionID: q.SessionID, RunID: q.RunID, ToolUseID: q.ToolUseID}); err != nil {
		return QuestionTransition{}, err
	}
	q.Status, q.Answers = "pending", nil
	q.CreatedAt = time.Now().UTC()
	q.UpdatedAt = q.CreatedAt
	event, err := saveQuestionTx(ctx, tx, q)
	if err != nil {
		return QuestionTransition{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return QuestionTransition{}, err
	}
	return QuestionTransition{Question: q, Events: []SessionEvent{event}, Changed: true}, nil
}

// GetQuestion returns a question owned by the session.
func (s *SQLiteStore) GetQuestion(ctx context.Context, sessionID, id string) (question.Question, error) {
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT payload_json FROM questions WHERE session_id = ? AND id = ?`, sessionID, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return question.Question{}, question.ErrNotFound
	}
	var q question.Question
	if err == nil {
		err = json.Unmarshal([]byte(data), &q)
	}
	return q, err
}

// ListQuestions returns all questions in creation order.
func (s *SQLiteStore) ListQuestions(ctx context.Context, sessionID string) ([]question.Question, error) {
	if _, err := s.GetSession(sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM questions WHERE session_id = ?`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []question.Question{}
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var q question.Question
		if err := json.Unmarshal([]byte(data), &q); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	SortQuestions(out)
	return out, rows.Err()
}

// ResolveQuestion commits a reply and wakes a suspended run when all its questions are resolved.
func (s *SQLiteStore) ResolveQuestion(ctx context.Context, sessionID, id string, reply question.Reply) (QuestionTransition, error) {
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return QuestionTransition{}, err
	}
	defer tx.Rollback()
	var data string
	err = tx.QueryRowContext(ctx, `SELECT payload_json FROM questions WHERE id = ? AND session_id = ?`, id, sessionID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return QuestionTransition{}, question.ErrNotFound
	}
	if err != nil {
		return QuestionTransition{}, err
	}
	var q question.Question
	if err := json.Unmarshal([]byte(data), &q); err != nil {
		return QuestionTransition{}, err
	}
	q, changed, err := question.Resolve(q, reply)
	if err != nil || !changed {
		return QuestionTransition{Question: q}, err
	}
	event, err := saveQuestionTx(ctx, tx, q)
	if err != nil {
		return QuestionTransition{}, err
	}
	events := []SessionEvent{event}
	if q.RunID != "" {
		run, err := scanSessionRun(tx.QueryRowContext(ctx, `SELECT `+sessionRunColumns+` FROM session_runs WHERE id = ?`, q.RunID))
		if err != nil {
			return QuestionTransition{}, err
		}
		if run.Status == SessionRunStatusAwaitingInput {
			var pending int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM questions WHERE run_id = ? AND status = 'pending'`, run.ID).Scan(&pending); err != nil {
				return QuestionTransition{}, err
			}
			if pending == 0 {
				run.Status = SessionRunStatusQueued
				run.UpdatedAt = time.Now().UTC()
				event, err := saveInputRunTx(ctx, tx, run)
				if err != nil {
					return QuestionTransition{}, err
				}
				events = append(events, event)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return QuestionTransition{}, err
	}
	return QuestionTransition{Question: q, Events: events, Changed: true}, nil
}

// SuspendSessionRunForQuestions releases a run only after its tool batch has stopped.
func (s *SQLiteStore) SuspendSessionRunForQuestions(ctx context.Context, sessionID, runID string) (SessionRunTransition, error) {
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return SessionRunTransition{}, err
	}
	defer tx.Rollback()
	run, err := scanSessionRun(tx.QueryRowContext(ctx, `SELECT `+sessionRunColumns+` FROM session_runs WHERE id = ? AND session_id = ?`, runID, sessionID))
	if errors.Is(err, sql.ErrNoRows) {
		return SessionRunTransition{}, ErrSessionRunNotFound
	}
	if err != nil {
		return SessionRunTransition{}, err
	}
	if run.Status != SessionRunStatusRunning {
		return SessionRunTransition{}, ErrSessionRunTransitionConflict
	}
	var pending int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM questions WHERE run_id = ? AND status = 'pending'`, runID).Scan(&pending); err != nil {
		return SessionRunTransition{}, err
	}
	run.Status = SessionRunStatusAwaitingInput
	// A client can answer before the tool batch reaches this boundary.
	if pending == 0 {
		run.Status = SessionRunStatusQueued
	}
	run.UpdatedAt = time.Now().UTC()
	event, err := saveInputRunTx(ctx, tx, run)
	if err != nil {
		return SessionRunTransition{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SessionRunTransition{}, err
	}
	return SessionRunTransition{Run: run, Event: event, Changed: true}, nil
}

func saveInputRunTx(ctx context.Context, tx *immediateTx, run SessionRun) (SessionEvent, error) {
	if _, err := tx.ExecContext(ctx, `UPDATE session_runs SET status = ?, updated_at = ? WHERE id = ?`, run.Status, formatTime(run.UpdatedAt), run.ID); err != nil {
		return SessionEvent{}, err
	}
	aggregate, err := NewSessionInputEvent(run)
	if err != nil {
		return SessionEvent{}, err
	}
	if err := appendQuestionAggregateTx(ctx, tx, aggregate); err != nil {
		return SessionEvent{}, err
	}
	return appendRunEventTx(ctx, tx, run, "session.run.input", nil, run.UpdatedAt)
}

func appendQuestionAggregateTx(ctx context.Context, tx *immediateTx, aggregate AggregateEvent) error {
	session, err := getSessionTx(ctx, tx, aggregate.Aggregate.ID)
	if err != nil {
		return err
	}
	aggregate, err = appendAggregateEventTx(ctx, tx, aggregate, session.AggregateVersion)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE sessions SET aggregate_version = ? WHERE id = ?`, aggregate.Version, session.ID)
	return err
}

func saveQuestionTx(ctx context.Context, tx *immediateTx, q question.Question) (SessionEvent, error) {
	payload, err := json.Marshal(q)
	if err != nil {
		return SessionEvent{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO questions (id, session_id, run_id, tool_use_id, status, payload_json) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET status = excluded.status, payload_json = excluded.payload_json`, q.ID, q.SessionID, nullableString(q.RunID), nullableString(q.ToolUseID), q.Status, string(payload)); err != nil {
		return SessionEvent{}, err
	}
	aggregate, err := NewSessionQuestionEvent(q)
	if err != nil {
		return SessionEvent{}, err
	}
	if err := appendQuestionAggregateTx(ctx, tx, aggregate); err != nil {
		return SessionEvent{}, err
	}
	var seq int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) + 1 FROM session_events WHERE session_id = ?`, q.SessionID).Scan(&seq); err != nil {
		return SessionEvent{}, err
	}
	event := SessionEvent{ID: NewID(PrefixEvent), SessionID: q.SessionID, Seq: seq, SchemaVersion: 1, Type: aggregate.Type, Time: q.UpdatedAt, DataJSON: payload, Data: payload}
	_, err = tx.ExecContext(ctx, `INSERT INTO session_events (id, session_id, seq, schema_version, type, data_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, event.ID, event.SessionID, seq, 1, event.Type, string(payload), formatTime(event.Time))
	return event, err
}

func cancelRunQuestionsTx(ctx context.Context, tx *immediateTx, runID string) error {
	rows, err := tx.QueryContext(ctx, `SELECT payload_json FROM questions WHERE run_id = ? AND status = 'pending'`, runID)
	if err != nil {
		return err
	}
	var questions []question.Question
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			rows.Close()
			return err
		}
		var q question.Question
		if err := json.Unmarshal([]byte(data), &q); err != nil {
			rows.Close()
			return err
		}
		questions = append(questions, q)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, q := range questions {
		q, _, err := question.Resolve(q, question.Reply{Status: "cancelled"})
		if err != nil {
			return err
		}
		if _, err := saveQuestionTx(ctx, tx, q); err != nil {
			return err
		}
	}
	rows, err = tx.QueryContext(ctx, `SELECT `+toolUseColumns+` FROM tool_uses WHERE run_id = ? AND status = ?`, runID, ToolUseStatusWaiting)
	if err != nil {
		return err
	}
	var uses []ToolUse
	for rows.Next() {
		use, err := scanToolUse(rows)
		if err != nil {
			rows.Close()
			return err
		}
		uses = append(uses, use)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, use := range uses {
		use.Status, use.ErrorType, use.ErrorMessage = ToolUseStatusInterrupted, "run_ended", "run ended while waiting for an answer"
		use.CompletedAt, use.UpdatedAt = time.Now().UTC(), time.Now().UTC()
		if err := updateToolUse(ctx, tx, use); err != nil {
			return err
		}
		if err := appendToolUseAggregateTx(ctx, tx, use); err != nil {
			return err
		}
	}
	return nil
}
