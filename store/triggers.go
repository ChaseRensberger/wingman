package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/chaserensberger/wingman/trigger"
)

// TriggerStore is the optional durable scheduling capability of a store.
type TriggerStore interface {
	SaveTrigger(context.Context, trigger.Trigger, int64) (trigger.Trigger, error)
	GetTrigger(context.Context, string, string) (trigger.Trigger, error)
	ListTriggers(context.Context, string) ([]trigger.Trigger, error)
	DeleteTrigger(context.Context, string, string, int64) error
	DueTriggers(context.Context, time.Time) ([]trigger.Trigger, error)
	ListTriggerOccurrences(context.Context, string, string, int) ([]trigger.Occurrence, error)
	GetTriggerOccurrence(context.Context, string, string, string) (*trigger.Occurrence, error)
	CommitTriggerOccurrence(context.Context, TriggerSubmission) (TriggerSubmissionResult, error)
}

// TriggerSubmission atomically records an occurrence and optionally admits its Run.
type TriggerSubmission struct {
	Trigger    trigger.Trigger
	Occurrence trigger.Occurrence
	NextFireAt time.Time
	Session    Session
	Run        SessionRun
}

// TriggerSubmissionResult includes the normal admission event for publication.
type TriggerSubmissionResult struct {
	Occurrence trigger.Occurrence
	Admission  SessionRunAdmission
}

const triggerColumns = `id, client_id, config_json, version, next_fire_at, created_at, updated_at`

func scanTrigger(row interface{ Scan(...any) error }) (trigger.Trigger, error) {
	var t trigger.Trigger
	var config, created, updated string
	var next sql.NullInt64
	if err := row.Scan(&t.ID, &t.ClientID, &config, &t.Version, &next, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return t, trigger.ErrNotFound
		}
		return t, err
	}
	if err := json.Unmarshal([]byte(config), &t.Config); err != nil {
		return t, err
	}
	var err error
	if t.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return t, err
	}
	if t.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return t, err
	}
	if next.Valid {
		value := time.UnixMilli(next.Int64).UTC()
		t.NextFireAt = &value
	}
	return t, nil
}

// SaveTrigger creates or replaces a definition with optimistic concurrency.
func (s *SQLiteStore) SaveTrigger(ctx context.Context, t trigger.Trigger, expected int64) (trigger.Trigger, error) {
	config, err := json.Marshal(t.Config)
	if err != nil {
		return t, err
	}
	var next any
	if t.NextFireAt != nil && t.Enabled {
		next = t.NextFireAt.UnixMilli()
	}
	t.UpdatedAt = time.Now().UTC()
	if expected == 0 {
		t.ID, t.Version, t.CreatedAt = NewID(PrefixTrigger), 1, t.UpdatedAt
		_, err = s.db.ExecContext(ctx, `INSERT INTO triggers (`+triggerColumns+`, enabled) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			t.ID, t.ClientID, string(config), t.Version, next, t.CreatedAt.Format(time.RFC3339Nano), t.UpdatedAt.Format(time.RFC3339Nano), t.Enabled)
	} else {
		t.Version = expected + 1
		var result sql.Result
		result, err = s.db.ExecContext(ctx, `UPDATE triggers SET config_json = ?, version = ?, enabled = ?, next_fire_at = ?, updated_at = ? WHERE id = ? AND client_id = ? AND version = ?`,
			string(config), t.Version, t.Enabled, next, t.UpdatedAt.Format(time.RFC3339Nano), t.ID, t.ClientID, expected)
		if err == nil {
			if n, rowsErr := result.RowsAffected(); rowsErr != nil {
				err = rowsErr
			} else if n != 1 {
				err = trigger.ErrConflict
			}
		}
	}
	if err != nil {
		return trigger.Trigger{}, err
	}
	return s.GetTrigger(ctx, t.ClientID, t.ID)
}

// GetTrigger returns a definition within its owning Client.
func (s *SQLiteStore) GetTrigger(ctx context.Context, clientID, id string) (trigger.Trigger, error) {
	t, err := scanTrigger(s.db.QueryRowContext(ctx, `SELECT `+triggerColumns+` FROM triggers WHERE id = ? AND client_id = ?`, id, clientID))
	if err != nil {
		return t, err
	}
	return s.triggerSummary(ctx, t)
}

func (s *SQLiteStore) triggerSummary(ctx context.Context, t trigger.Trigger) (trigger.Trigger, error) {
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM trigger_occurrences WHERE trigger_id = ? AND run_id != ''`, t.ID).Scan(&t.RunCount); err != nil {
		return t, err
	}
	items, err := s.ListTriggerOccurrences(ctx, t.ClientID, t.ID, 1)
	if len(items) != 0 {
		t.LastOccurrence = &items[0]
	}
	return t, err
}

// ListTriggers returns client-owned definitions with their latest outcome.
func (s *SQLiteStore) ListTriggers(ctx context.Context, clientID string) ([]trigger.Trigger, error) {
	items, err := s.readTriggers(ctx, `SELECT `+triggerColumns+` FROM triggers WHERE client_id = ? ORDER BY created_at DESC, id`, clientID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i], err = s.triggerSummary(ctx, items[i])
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (s *SQLiteStore) readTriggers(ctx context.Context, query string, args ...any) ([]trigger.Trigger, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []trigger.Trigger{}
	for rows.Next() {
		t, err := scanTrigger(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

// DeleteTrigger removes future scheduling and occurrence history, but keeps Sessions.
func (s *SQLiteStore) DeleteTrigger(ctx context.Context, clientID, id string, expected int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM triggers WHERE id = ? AND client_id = ? AND version = ?`, id, clientID, expected)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return trigger.ErrConflict
	}
	return err
}

// DueTriggers returns enabled definitions whose next occurrence is due.
func (s *SQLiteStore) DueTriggers(ctx context.Context, now time.Time) ([]trigger.Trigger, error) {
	return s.readTriggers(ctx, `SELECT `+triggerColumns+` FROM triggers WHERE enabled = 1 AND next_fire_at <= ? ORDER BY next_fire_at, id`, now.UnixMilli())
}

const occurrenceColumns = `o.id, o.trigger_id, o.trigger_name, o.source, o.request_id, o.scheduled_at, o.created_at,
	CASE WHEN o.status = 'admitted' THEN COALESCE(r.status, 'deleted') ELSE o.status END,
	COALESCE(NULLIF(r.error_message, ''), o.reason), o.session_id, o.run_id, r.started_at, r.completed_at`

func scanOccurrence(row interface{ Scan(...any) error }) (trigger.Occurrence, error) {
	var o trigger.Occurrence
	var scheduled, created string
	var started, completed sql.NullString
	if err := row.Scan(&o.ID, &o.TriggerID, &o.TriggerName, &o.Source, &o.RequestID, &scheduled, &created, &o.Status, &o.Reason, &o.SessionID, &o.RunID, &started, &completed); err != nil {
		return o, err
	}
	var err error
	if o.ScheduledAt, err = time.Parse(time.RFC3339Nano, scheduled); err != nil {
		return o, err
	}
	if o.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return o, err
	}
	for _, item := range []struct {
		value  sql.NullString
		target **time.Time
	}{{started, &o.StartedAt}, {completed, &o.CompletedAt}} {
		if item.value.Valid && item.value.String != "" {
			value, err := time.Parse(time.RFC3339Nano, item.value.String)
			if err != nil {
				return o, err
			}
			*item.target = &value
		}
	}
	return o, nil
}

// ListTriggerOccurrences returns recent client-owned activity, optionally for one trigger.
func (s *SQLiteStore) ListTriggerOccurrences(ctx context.Context, clientID, triggerID string, limit int) ([]trigger.Occurrence, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+occurrenceColumns+` FROM trigger_occurrences o
		JOIN triggers t ON t.id = o.trigger_id LEFT JOIN session_runs r ON r.id = o.run_id
		WHERE t.client_id = ? AND (? = '' OR t.id = ?) ORDER BY o.created_at DESC, o.id DESC LIMIT ?`, clientID, triggerID, triggerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []trigger.Occurrence{}
	for rows.Next() {
		o, err := scanOccurrence(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, o)
	}
	return items, rows.Err()
}

// GetTriggerOccurrence resolves a previously recorded submission identity.
func (s *SQLiteStore) GetTriggerOccurrence(ctx context.Context, id, source, requestID string) (*trigger.Occurrence, error) {
	o, err := scanOccurrence(s.db.QueryRowContext(ctx, `SELECT `+occurrenceColumns+` FROM trigger_occurrences o LEFT JOIN session_runs r ON r.id = o.run_id WHERE o.trigger_id = ? AND o.source = ? AND o.request_id = ?`, id, source, requestID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &o, err
}

// CommitTriggerOccurrence commits the cursor, occurrence, fresh Session, and Run together.
func (s *SQLiteStore) CommitTriggerOccurrence(ctx context.Context, submission TriggerSubmission) (TriggerSubmissionResult, error) {
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return TriggerSubmissionResult{}, err
	}
	defer tx.Rollback()
	t, o := submission.Trigger, submission.Occurrence
	existing, err := scanOccurrence(tx.QueryRowContext(ctx, `SELECT `+occurrenceColumns+` FROM trigger_occurrences o LEFT JOIN session_runs r ON r.id = o.run_id WHERE o.trigger_id = ? AND o.source = ? AND o.request_id = ?`, t.ID, o.Source, o.RequestID))
	if err == nil {
		return TriggerSubmissionResult{Occurrence: existing}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return TriggerSubmissionResult{}, err
	}
	current, err := scanTrigger(tx.QueryRowContext(ctx, `SELECT `+triggerColumns+` FROM triggers WHERE id = ? AND client_id = ?`, t.ID, t.ClientID))
	if err != nil {
		return TriggerSubmissionResult{}, err
	}
	if current.Version != t.Version {
		return TriggerSubmissionResult{}, trigger.ErrConflict
	}
	if o.Source == "cron" {
		if !current.Enabled || current.NextFireAt == nil || !current.NextFireAt.Equal(o.ScheduledAt) {
			return TriggerSubmissionResult{}, trigger.ErrConflict
		}
		if _, err := tx.ExecContext(ctx, `UPDATE triggers SET next_fire_at = ? WHERE id = ?`, submission.NextFireAt.UnixMilli(), t.ID); err != nil {
			return TriggerSubmissionResult{}, err
		}
	}
	if o.Status == "" {
		var active int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM trigger_occurrences o JOIN session_runs r ON r.id = o.run_id WHERE o.trigger_id = ? AND r.status IN ('queued', 'running')`, t.ID).Scan(&active); err != nil {
			return TriggerSubmissionResult{}, err
		}
		if active > 0 {
			o.Status, o.Reason = "skipped", "The previous run is still queued or running."
		}
	}
	result := TriggerSubmissionResult{}
	if o.Status == "" {
		sess := submission.Session
		if err := createSessionTx(ctx, tx, &sess); err != nil {
			return result, err
		}
		run := submission.Run
		run.SessionID, run.RequestID = sess.ID, o.RequestID
		result.Admission, err = admitSessionRunTx(ctx, tx, run)
		if err != nil {
			return result, err
		}
		o.Status, o.SessionID, o.RunID = "admitted", sess.ID, result.Admission.Run.ID
	}
	o.ID, o.TriggerID, o.TriggerName = NewID(PrefixOccurrence), t.ID, t.Name
	o.CreatedAt = time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO trigger_occurrences
		(id, trigger_id, trigger_name, source, request_id, scheduled_at, created_at, status, reason, session_id, run_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, o.ID, o.TriggerID, o.TriggerName, o.Source, o.RequestID,
		o.ScheduledAt.UTC().Format(time.RFC3339Nano), o.CreatedAt.Format(time.RFC3339Nano), o.Status, o.Reason, o.SessionID, o.RunID); err != nil {
		return result, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TriggerSubmissionResult{}, err
	}
	if o.Status == "admitted" {
		o.Status = result.Admission.Run.Status
	}
	result.Occurrence = o
	return result, nil
}
