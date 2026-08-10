// Package storage owns agent's SQLite-backed persistence: agents,
// sessions, message history, and provider auth credentials.
//
// Schema lives in ./migrations as numbered .sql files; runMigrations
// applies them at open time. IDs are KSUID strings prefixed with a
// typed tag (see id.go).
//
// Concurrency: SQLite under modernc.org/sqlite is configured with WAL
// for readers/writer concurrency, but we still cap MaxOpenConns to 1 to
// serialize all writes through a single connection. v0.1 is single-process;
// we revisit pool sizing if the daemon ever fans out.
package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore is the concrete persistence layer. Construct with
// NewSQLiteStore; share a single instance across the process.
type SQLiteStore struct {
	db *sql.DB
}

type immediateTx struct {
	conn *sql.Conn
	done bool
}

func (s *SQLiteStore) beginImmediate(ctx context.Context) (*immediateTx, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &immediateTx{conn: conn}, nil
}

func (tx *immediateTx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return tx.conn.QueryRowContext(ctx, query, args...)
}

func (tx *immediateTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return tx.conn.QueryContext(ctx, query, args...)
}

func (tx *immediateTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return tx.conn.ExecContext(ctx, query, args...)
}

func (tx *immediateTx) Commit(ctx context.Context) error {
	if tx.done {
		return errors.New("transaction already closed")
	}
	_, err := tx.conn.ExecContext(ctx, `COMMIT`)
	if err != nil {
		_, _ = tx.conn.ExecContext(context.Background(), `ROLLBACK`)
	}
	tx.done = true
	closeErr := tx.conn.Close()
	return errors.Join(err, closeErr)
}

func (tx *immediateTx) Rollback() {
	if tx.done {
		return
	}
	_, _ = tx.conn.ExecContext(context.Background(), `ROLLBACK`)
	tx.done = true
	_ = tx.conn.Close()
}

// NewSQLiteStore opens (and creates if missing) a SQLite DB at dbPath,
// applies pragmas for durability+concurrency, and runs all pending
// migrations.
//
// Pragma rationale:
//   - journal_mode=WAL: concurrent reads while a writer is active. Standard
//     for write-heavy SQLite.
//   - synchronous=NORMAL: fsync only at WAL checkpoint (every 1000 pages
//     by default). Slightly less durable than FULL on power loss but ~10x
//     faster; acceptable for a developer tool.
//   - foreign_keys=ON: enforce ON DELETE CASCADE we declared in the schema.
//     SQLite ships this OFF by default.
//   - busy_timeout=5000: wait up to 5s on lock contention before erroring.
//     Smooths over short writer queues.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	_, statErr := os.Stat(dbPath)
	dbExists := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("stat database %s: %w", dbPath, statErr)
	}

	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create directory %s: %w", dir, err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Pragmas must run before MaxOpenConns clamps us, because pragmas
	// are per-connection. With MaxOpenConns(1) every later query reuses
	// the configured connection.
	for _, p := range []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA synchronous = NORMAL`,
		`PRAGMA foreign_keys = ON`,
		`PRAGMA busy_timeout = 5000`,
	} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("pragma %q: %w", p, err)
		}
	}
	db.SetMaxOpenConns(1)

	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrations: %w", err)
	}

	store := &SQLiteStore{db: db}
	if err := store.validateSessionAggregateCompatibility(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("validate session aggregate compatibility: %w", err)
	}
	if !dbExists {
		if err := store.seedDefaultAgents(); err != nil {
			db.Close()
			return nil, fmt.Errorf("seed default agents: %w", err)
		}
	}

	return store, nil
}

// RebuildSessionProjections replaces one Session aggregate's derived state
// from its immutable aggregate history. Aggregate and public session events
// are intentionally left untouched.
func (s *SQLiteStore) RebuildSessionProjections(ctx context.Context, sessionID string) error {
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	projection, err := projectSQLiteSessionAggregate(ctx, tx, sessionID)
	if err != nil {
		return fmt.Errorf("rebuild session projections %s: %w", sessionID, err)
	}
	if err := replaceSQLiteSessionProjection(ctx, tx, projection); err != nil {
		return fmt.Errorf("rebuild session projections %s: %w", sessionID, err)
	}
	return tx.Commit(ctx)
}

// RebuildAllSessionProjections replaces every Session aggregate's derived
// state in one transaction. Aggregate and public session events are preserved.
func (s *SQLiteStore) RebuildAllSessionProjections(ctx context.Context) error {
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ids, err := sqliteSessionAggregateIDs(ctx, tx)
	if err != nil {
		return err
	}
	projections := make([]SessionAggregateProjection, 0, len(ids))
	for _, id := range ids {
		projection, err := projectSQLiteSessionAggregate(ctx, tx, id)
		if err != nil {
			return fmt.Errorf("rebuild session projections %s: %w", id, err)
		}
		projections = append(projections, projection)
	}
	for _, projection := range projections {
		if err := replaceSQLiteSessionProjection(ctx, tx, projection); err != nil {
			return fmt.Errorf("replace session projection %s: %w", projection.Session.ID, err)
		}
	}
	return tx.Commit(ctx)
}

func (s *SQLiteStore) validateSessionAggregateCompatibility(ctx context.Context) error {
	ids, err := sqliteSessionAggregateIDs(ctx, s.db)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := projectSQLiteSessionAggregate(ctx, s.db, id); err != nil {
			return fmt.Errorf("session %s: %w", id, err)
		}
	}
	return nil
}

type aggregateEventQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func sqliteSessionAggregateIDs(ctx context.Context, q aggregateEventQueryer) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT DISTINCT aggregate_id FROM aggregate_events WHERE aggregate_type = ? ORDER BY aggregate_id`, AggregateSession)
	if err != nil {
		return nil, fmt.Errorf("list session aggregates: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan session aggregate: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func projectSQLiteSessionAggregate(ctx context.Context, q aggregateEventQueryer, sessionID string) (SessionAggregateProjection, error) {
	rows, err := q.QueryContext(ctx, `SELECT global_sequence, id, version, event_type, schema_version, payload_json, created_at, COALESCE(causation_id, ''), COALESCE(correlation_id, ''), COALESCE(client_id, ''), COALESCE(run_id, '') FROM aggregate_events WHERE aggregate_type = ? AND aggregate_id = ? ORDER BY version`, AggregateSession, sessionID)
	if err != nil {
		return SessionAggregateProjection{}, fmt.Errorf("read aggregate history: %w", err)
	}
	defer rows.Close()
	events := []AggregateEvent{}
	for rows.Next() {
		var event AggregateEvent
		var payload []byte
		var createdAt string
		event.Aggregate = AggregateRef{Type: AggregateSession, ID: sessionID}
		if err := rows.Scan(&event.GlobalSequence, &event.ID, &event.Version, &event.Type, &event.SchemaVersion, &payload, &createdAt, &event.CausationID, &event.CorrelationID, &event.ClientID, &event.RunID); err != nil {
			return SessionAggregateProjection{}, fmt.Errorf("scan aggregate event: %w", err)
		}
		event.Data = append(json.RawMessage(nil), payload...)
		if event.Time, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return SessionAggregateProjection{}, fmt.Errorf("parse aggregate event time: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return SessionAggregateProjection{}, err
	}
	return ProjectSessionAggregate(events)
}

func replaceSQLiteSessionProjection(ctx context.Context, tx *immediateTx, projection SessionAggregateProjection) error {
	id := projection.Session.ID
	for _, table := range []string{"permission_requests", "permission_grants", "tool_uses", "model_calls"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE session_id = ?`, id); err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM parts WHERE message_id IN (SELECT id FROM messages WHERE session_id = ?)`, id); err != nil {
		return fmt.Errorf("clear parts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE session_id = ?`, id); err != nil {
		return fmt.Errorf("clear messages: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM session_runs WHERE session_id = ?`, id); err != nil {
		return fmt.Errorf("clear session runs: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sessions (id, title, work_dir, workspace_id, client_id, created_at, updated_at, aggregate_version) VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?) ON CONFLICT(id) DO UPDATE SET title = excluded.title, work_dir = excluded.work_dir, workspace_id = excluded.workspace_id, client_id = excluded.client_id, created_at = excluded.created_at, updated_at = excluded.updated_at, aggregate_version = excluded.aggregate_version`, id, projection.Session.Title, projection.Session.WorkDir, projection.Session.WorkspaceID, projection.Session.ClientID, projection.Session.CreatedAt, projection.Session.UpdatedAt, projection.Session.AggregateVersion); err != nil {
		return fmt.Errorf("replace session: %w", err)
	}
	for _, run := range projection.Runs {
		agent, err := json.Marshal(run.Agent)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO session_runs (id, session_id, request_id, request_hash, admitted_version, work_dir, workspace_id, client_id, sequence, status, message, agent_json, output_schema_json, error_type, error_message, created_at, started_at, completed_at, updated_at) VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?)`, run.ID, run.SessionID, run.RequestID, run.RequestHash, run.AdmittedVersion, run.WorkDir, run.WorkspaceID, run.ClientID, run.Sequence, run.Status, run.Message, string(agent), nullableBytes(run.OutputSchemaJSON), run.ErrorType, run.ErrorMessage, formatTime(run.CreatedAt), nullableTime(run.StartedAt), nullableTime(run.CompletedAt), formatTime(run.UpdatedAt)); err != nil {
			return fmt.Errorf("insert session run: %w", err)
		}
	}
	for _, message := range projection.Messages {
		if err := insertMessageTx(ctx, tx, message, message.CreatedAt); err != nil {
			return err
		}
	}
	for _, call := range projection.ModelCalls {
		if _, err := tx.ExecContext(ctx, `INSERT INTO model_calls (id, session_id, run_id, assistant_message_id, step, attempt, status, agent_id, model_ref, provider, provider_request_id, api, model_id, finish_reason, stop_reason, error_type, error_message, input_tokens, output_tokens, reasoning_tokens, cached_input_tokens, cache_write_tokens, total_tokens, context_tokens, context_window, context_percent, cost, structured_output_json, metadata_json, started_at, completed_at, created_at, updated_at) VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, call.ID, call.SessionID, call.RunID, call.AssistantMessageID, call.Step, call.Attempt, call.Status, call.AgentID, call.ModelRef, call.Provider, call.ProviderRequestID, call.API, call.ModelID, call.FinishReason, call.StopReason, call.ErrorType, call.ErrorMessage, call.InputTokens, call.OutputTokens, call.ReasoningTokens, call.CachedInputTokens, call.CacheWriteTokens, call.TotalTokens, call.ContextTokens, call.ContextWindow, call.ContextPercent, call.Cost, nullableBytes(call.StructuredOutputJSON), nullableBytes(call.MetadataJSON), formatTime(call.StartedAt), nullableTime(call.CompletedAt), formatTime(call.CreatedAt), formatTime(call.UpdatedAt)); err != nil {
			return fmt.Errorf("insert model call: %w", err)
		}
	}
	for _, use := range projection.ToolUses {
		if err := insertToolUse(ctx, tx, use); err != nil {
			return fmt.Errorf("insert tool use: %w", err)
		}
	}
	for _, request := range projection.PermissionRequests {
		resources, err := json.Marshal(request.Resources)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO permission_requests (id, session_id, run_id, tool_use_id, call_id, action, resources_json, status, response, error_type, error_message, created_at, resolved_at, updated_at) VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, request.ID, request.SessionID, request.RunID, request.ToolUseID, request.CallID, request.Action, string(resources), request.Status, request.Response, request.ErrorType, request.ErrorMessage, formatTime(request.CreatedAt), nullableTime(request.ResolvedAt), formatTime(request.UpdatedAt)); err != nil {
			return fmt.Errorf("insert permission request: %w", err)
		}
	}
	for _, grant := range projection.PermissionGrants {
		if _, err := tx.ExecContext(ctx, `INSERT INTO permission_grants (id, session_id, action, resource, created_at) VALUES (?, ?, ?, ?, ?)`, grant.ID, grant.SessionID, grant.Action, grant.Resource, formatTime(grant.CreatedAt)); err != nil {
			return fmt.Errorf("insert permission grant: %w", err)
		}
	}
	return nil
}

// Close releases the underlying database handle.
func (s *SQLiteStore) Close() error { return s.db.Close() }

type aggregateEventTx interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func appendAggregateEventTx(ctx context.Context, tx aggregateEventTx, event AggregateEvent, expectedVersion int64) (AggregateEvent, error) {
	var actualVersion int64
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version), 0)
		FROM aggregate_events
		WHERE aggregate_type = ? AND aggregate_id = ?
	`, event.Aggregate.Type, event.Aggregate.ID).Scan(&actualVersion); err != nil {
		return AggregateEvent{}, fmt.Errorf("read aggregate version: %w", err)
	}
	if actualVersion != expectedVersion {
		return AggregateEvent{}, &AggregateVersionConflict{
			Aggregate: event.Aggregate,
			Expected:  expectedVersion,
			Actual:    actualVersion,
		}
	}
	if event.ID == "" {
		event.ID = NewID(PrefixEvent)
	}
	if event.SchemaVersion == 0 {
		event.SchemaVersion = 1
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	if len(event.Data) == 0 {
		event.Data = json.RawMessage(`{}`)
	}
	event.Version = expectedVersion + 1
	result, err := tx.ExecContext(ctx, `
		INSERT INTO aggregate_events (
			id, aggregate_type, aggregate_id, version, event_type,
			schema_version, payload_json, created_at, causation_id,
			correlation_id, client_id, run_id
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''))
	`, event.ID, event.Aggregate.Type, event.Aggregate.ID, event.Version, event.Type,
		event.SchemaVersion, []byte(event.Data), event.Time.UTC().Format(time.RFC3339Nano),
		event.CausationID, event.CorrelationID, event.ClientID, event.RunID)
	if err != nil {
		return AggregateEvent{}, fmt.Errorf("append aggregate event: %w", err)
	}
	event.GlobalSequence, err = result.LastInsertId()
	if err != nil {
		return AggregateEvent{}, fmt.Errorf("read aggregate global sequence: %w", err)
	}
	return event, nil
}

// ListAggregateEvents returns an aggregate's immutable events in version order.
func (s *SQLiteStore) ListAggregateEvents(ctx context.Context, aggregate AggregateRef, afterVersion int64, limit int) ([]AggregateEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT global_sequence, id, aggregate_type, aggregate_id, version,
			event_type, schema_version, payload_json, created_at,
			COALESCE(causation_id, ''), COALESCE(correlation_id, ''),
			COALESCE(client_id, ''), COALESCE(run_id, '')
		FROM aggregate_events
		WHERE aggregate_type = ? AND aggregate_id = ? AND version > ?
		ORDER BY version ASC
		LIMIT ?
	`, aggregate.Type, aggregate.ID, afterVersion, limit)
	if err != nil {
		return nil, fmt.Errorf("list aggregate events: %w", err)
	}
	defer rows.Close()
	events := make([]AggregateEvent, 0)
	for rows.Next() {
		var event AggregateEvent
		var aggregateType string
		var payload []byte
		var createdAt string
		if err := rows.Scan(
			&event.GlobalSequence, &event.ID, &aggregateType, &event.Aggregate.ID,
			&event.Version, &event.Type, &event.SchemaVersion, &payload, &createdAt,
			&event.CausationID, &event.CorrelationID, &event.ClientID, &event.RunID,
		); err != nil {
			return nil, fmt.Errorf("scan aggregate event: %w", err)
		}
		event.Aggregate.Type = AggregateType(aggregateType)
		event.Data = append(json.RawMessage(nil), payload...)
		event.Time, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse aggregate event time: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list aggregate events: %w", err)
	}
	return events, nil
}

// DefaultDBPath returns the platform-appropriate default DB location.
func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "wingman", "wingman.db"), nil
}

// Now returns the current UTC timestamp formatted as RFC3339.
func Now() string { return time.Now().UTC().Format(time.RFC3339) }

// ---- agents --------------------------------------------------------------

func (s *SQLiteStore) seedDefaultAgents() error {
	for _, agent := range DefaultAgents() {
		if err := s.CreateAgent(agent); err != nil {
			return err
		}
	}
	return nil
}

// CreateAgent inserts a new agent row. If agent.ID is empty, a fresh
// KSUID is minted. CreatedAt/UpdatedAt are always overwritten with Now().
func (s *SQLiteStore) CreateAgent(agent *Agent) error {
	if agent.ID == "" {
		agent.ID = NewID(PrefixAgent)
	}
	now := Now()
	agent.CreatedAt = now
	agent.UpdatedAt = now

	tools, err := json.Marshal(agent.Tools)
	if err != nil {
		return err
	}
	permissionsJSON, err := marshalNullable(agent.Permissions)
	if err != nil {
		return err
	}

	optionsJSON, err := marshalNullable(agent.Options)
	if err != nil {
		return err
	}
	outputSchemaJSON, err := marshalNullable(agent.OutputSchema)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
		INSERT INTO agents (id, name, instructions, tools_json, permissions_json, model_ref, options_json, output_schema_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, agent.ID, agent.Name, agent.Instructions, string(tools), permissionsJSON, agent.ModelRef, optionsJSON, outputSchemaJSON, agent.CreatedAt, agent.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert agent: %w", err)
	}
	return nil
}

// GetAgent returns the agent with the given ID, or an error if not found.
func (s *SQLiteStore) GetAgent(id string) (*Agent, error) {
	row := s.db.QueryRow(`
		SELECT id, name, instructions, tools_json, permissions_json, model_ref, options_json, output_schema_json, created_at, updated_at
		FROM agents WHERE id = ?
	`, id)
	a, err := scanAgent(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent not found: %s", id)
	}
	return a, err
}

// ListAgents returns every agent, newest first by created_at.
func (s *SQLiteStore) ListAgents() ([]*Agent, error) {
	rows, err := s.db.Query(`
		SELECT id, name, instructions, tools_json, permissions_json, model_ref, options_json, output_schema_json, created_at, updated_at
		FROM agents ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpdateAgent overwrites the agent's mutable fields. Returns an error if
// the row does not exist.
func (s *SQLiteStore) UpdateAgent(agent *Agent) error {
	agent.UpdatedAt = Now()

	tools, err := json.Marshal(agent.Tools)
	if err != nil {
		return err
	}
	permissionsJSON, err := marshalNullable(agent.Permissions)
	if err != nil {
		return err
	}
	optionsJSON, err := marshalNullable(agent.Options)
	if err != nil {
		return err
	}
	outputSchemaJSON, err := marshalNullable(agent.OutputSchema)
	if err != nil {
		return err
	}

	res, err := s.db.Exec(`
		UPDATE agents SET name = ?, instructions = ?, tools_json = ?, permissions_json = ?, model_ref = ?, options_json = ?, output_schema_json = ?, updated_at = ?
		WHERE id = ?
	`, agent.Name, agent.Instructions, string(tools), permissionsJSON, agent.ModelRef, optionsJSON, outputSchemaJSON, agent.UpdatedAt, agent.ID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("agent not found: %s", agent.ID)
	}
	return nil
}

// DeleteAgent removes the agent. Returns an error if not found. Does NOT
// cascade to sessions (sessions reference agents only at runtime).
func (s *SQLiteStore) DeleteAgent(id string) error {
	res, err := s.db.Exec(`DELETE FROM agents WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("agent not found: %s", id)
	}
	return nil
}

// ---- clients -------------------------------------------------------------

// CreateClient inserts a new Wingman API client row with a fresh KSUID and the
// current RFC3339 timestamp.
func (s *SQLiteStore) CreateClient(name string) (*Client, error) {
	return s.CreateClientWithID(NewID(PrefixClient), name)
}

// CreateClientWithID inserts a client with its caller-provided stable ID.
func (s *SQLiteStore) CreateClientWithID(id, name string) (*Client, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("client name is required")
	}
	if strings.EqualFold(name, DefaultClientName) {
		return nil, ErrClientNameExists
	}
	clients, err := s.ListClients()
	if err != nil {
		return nil, err
	}
	for _, existing := range clients {
		if strings.EqualFold(existing.Name, name) {
			return nil, ErrClientNameExists
		}
	}

	client := &Client{
		ID:        id,
		Name:      name,
		CreatedAt: Now(),
	}
	_, err = s.db.Exec(`
		INSERT INTO clients (id, name, created_at) VALUES (?, ?, ?)
	`, client.ID, client.Name, client.CreatedAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "clients.id") {
			return nil, ErrClientIDExists
		}
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrClientNameExists
		}
		return nil, fmt.Errorf("insert client: %w", err)
	}
	return client, nil
}

// EnsureDefaultClient creates the built-in Wingman client if needed and returns it.
func (s *SQLiteStore) EnsureDefaultClient() (*Client, error) {
	now := Now()
	if _, err := s.db.Exec(`
		INSERT INTO clients (id, name, created_at) VALUES (?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, DefaultClientID, DefaultClientName, now); err != nil {
		return nil, fmt.Errorf("ensure default client: %w", err)
	}
	client, err := s.GetClient(DefaultClientID)
	if err != nil {
		return nil, err
	}
	if client.Name != DefaultClientName {
		return nil, errors.New("default client name is reserved")
	}
	return client, nil
}

// GetClient returns the client with the given ID, or an error if not found.
func (s *SQLiteStore) GetClient(id string) (*Client, error) {
	var c Client
	err := s.db.QueryRow(`
		SELECT id, name, created_at FROM clients WHERE id = ?
	`, id).Scan(&c.ID, &c.Name, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("client not found: %s", id)
	}
	return &c, err
}

// ListClients returns every client, newest first by created_at.
func (s *SQLiteStore) ListClients() ([]*Client, error) {
	rows, err := s.db.Query(`
		SELECT id, name, created_at FROM clients ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Client
	for rows.Next() {
		var c Client
		if err := rows.Scan(&c.ID, &c.Name, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// ---- workspaces ---------------------------------------------------------------

// CreateWorkspace inserts a saved working directory. If workspace.ID is empty, a
// fresh KSUID is minted. CreatedAt/UpdatedAt are always overwritten with Now().
func (s *SQLiteStore) CreateWorkspace(workspace *Workspace) error {
	if workspace.ID == "" {
		workspace.ID = NewID(PrefixWorkspace)
	}
	now := Now()
	workspace.CreatedAt = now
	workspace.UpdatedAt = now

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if workspace.ClientID != "" {
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM clients WHERE id = ?`, workspace.ClientID).Scan(&exists); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("client not found: %s", workspace.ClientID)
			}
			return fmt.Errorf("verify client: %w", err)
		}
	}
	if err := verifyWorkspaceNameAvailable(tx, workspace.ClientID, workspace.Name, ""); err != nil {
		return err
	}

	var clientIDPtr *string
	if workspace.ClientID != "" {
		clientIDPtr = &workspace.ClientID
	}
	if _, err := tx.Exec(`
		INSERT INTO workspaces (id, name, path, client_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, workspace.ID, workspace.Name, workspace.Path, clientIDPtr, workspace.CreatedAt, workspace.UpdatedAt); err != nil {
		return fmt.Errorf("insert workspace: %w", err)
	}

	return tx.Commit()
}

// GetWorkspace returns the workspace with the given ID, or an error if not found.
func (s *SQLiteStore) GetWorkspace(id string) (*Workspace, error) {
	var workspace Workspace
	var clientID sql.NullString
	err := s.db.QueryRow(`
		SELECT id, name, path, client_id, created_at, updated_at FROM workspaces WHERE id = ?
	`, id).Scan(&workspace.ID, &workspace.Name, &workspace.Path, &clientID, &workspace.CreatedAt, &workspace.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workspace not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	workspace.ClientID = clientID.String
	return &workspace, nil
}

// ListWorkspaces returns every workspace, newest first by created_at.
func (s *SQLiteStore) ListWorkspaces() ([]*Workspace, error) {
	rows, err := s.db.Query(`
		SELECT id, name, path, client_id, created_at, updated_at FROM workspaces ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanWorkspaces(rows)
}

// ListWorkspacesByClient returns every workspace attributed to a specific client.
func (s *SQLiteStore) ListWorkspacesByClient(clientID string) ([]*Workspace, error) {
	rows, err := s.db.Query(`
		SELECT id, name, path, client_id, created_at, updated_at FROM workspaces WHERE client_id = ? ORDER BY created_at DESC
	`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanWorkspaces(rows)
}

func scanWorkspaces(rows *sql.Rows) ([]*Workspace, error) {
	var out []*Workspace
	for rows.Next() {
		var workspace Workspace
		var clientID sql.NullString
		if err := rows.Scan(&workspace.ID, &workspace.Name, &workspace.Path, &clientID, &workspace.CreatedAt, &workspace.UpdatedAt); err != nil {
			return nil, err
		}
		workspace.ClientID = clientID.String
		out = append(out, &workspace)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateWorkspace overwrites the workspace's mutable fields.
func (s *SQLiteStore) UpdateWorkspace(workspace *Workspace) error {
	workspace.UpdatedAt = Now()
	if err := verifyWorkspaceNameAvailable(s.db, workspace.ClientID, workspace.Name, workspace.ID); err != nil {
		return err
	}
	var clientIDPtr *string
	if workspace.ClientID != "" {
		clientIDPtr = &workspace.ClientID
	}

	res, err := s.db.Exec(`
		UPDATE workspaces SET name = ?, path = ?, client_id = ?, updated_at = ? WHERE id = ?
	`, workspace.Name, workspace.Path, clientIDPtr, workspace.UpdatedAt, workspace.ID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("workspace not found: %s", workspace.ID)
	}
	return nil
}

type queryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

func verifyWorkspaceNameAvailable(q queryer, clientID, name, excludeID string) error {
	var exists int
	err := q.QueryRow(`
		SELECT 1 FROM workspaces
		WHERE COALESCE(client_id, '') = COALESCE(?, '')
			AND name = ? COLLATE NOCASE
			AND id != ?
	`, clientID, name, excludeID).Scan(&exists)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	return ErrWorkspaceNameExists
}

// DeleteWorkspace removes the workspace. Linked sessions keep their work_dir and
// have workspace_id set to NULL by the foreign key.
func (s *SQLiteStore) DeleteWorkspace(id string) error {
	res, err := s.db.Exec(`DELETE FROM workspaces WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("workspace not found: %s", id)
	}
	return nil
}

// ---- sessions ------------------------------------------------------------

// CreateSession appends session.created and updates the session projection in
// one transaction.
func (s *SQLiteStore) CreateSession(session *Session) error {
	if session.ID == "" {
		session.ID = NewID(PrefixSession)
	}
	now := Now()
	session.CreatedAt = now
	session.UpdatedAt = now

	ctx := context.Background()
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if session.ClientID != "" {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM clients WHERE id = ?`, session.ClientID).Scan(&exists); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("client not found: %s", session.ClientID)
			}
			return fmt.Errorf("verify client: %w", err)
		}
	}
	if session.WorkspaceID != "" {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM workspaces WHERE id = ?`, session.WorkspaceID).Scan(&exists); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("workspace not found: %s", session.WorkspaceID)
			}
			return fmt.Errorf("verify workspace: %w", err)
		}
	}
	event, err := NewSessionCreatedEvent(*session)
	if err != nil {
		return err
	}
	event, err = appendAggregateEventTx(ctx, tx, event, 0)
	if err != nil {
		return err
	}
	projected, err := ProjectSession([]AggregateEvent{event})
	if err != nil {
		return err
	}

	var workDirPtr *string
	if projected.WorkDir != "" {
		workDirPtr = &projected.WorkDir
	}
	var workspaceIDPtr *string
	if projected.WorkspaceID != "" {
		workspaceIDPtr = &projected.WorkspaceID
	}
	var clientIDPtr *string
	if projected.ClientID != "" {
		clientIDPtr = &projected.ClientID
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sessions (id, title, work_dir, workspace_id, client_id, created_at, updated_at, aggregate_version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, projected.ID, projected.Title, workDirPtr, workspaceIDPtr, clientIDPtr, projected.CreatedAt, projected.UpdatedAt, projected.AggregateVersion); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	*session = *projected
	return nil
}

// GetSession returns the session metadata.
func (s *SQLiteStore) GetSession(id string) (*Session, error) {
	var session Session
	var workDir sql.NullString
	var workspaceID sql.NullString
	var clientID sql.NullString
	err := s.db.QueryRow(`
		SELECT id, title, work_dir, workspace_id, client_id, created_at, updated_at, aggregate_version FROM sessions WHERE id = ?
	`, id).Scan(&session.ID, &session.Title, &workDir, &workspaceID, &clientID, &session.CreatedAt, &session.UpdatedAt, &session.AggregateVersion)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	session.WorkDir = workDir.String
	session.WorkspaceID = workspaceID.String
	session.ClientID = clientID.String
	return &session, nil
}

// ListSessions returns every session, newest first. History is no longer
// loaded automatically; use ListMessages for message retrieval.
func (s *SQLiteStore) ListSessions() ([]*Session, error) {
	rows, err := s.db.Query(`
		SELECT id, title, work_dir, workspace_id, client_id, created_at, updated_at, aggregate_version FROM sessions ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Session
	for rows.Next() {
		var sess Session
		var workDir sql.NullString
		var workspaceID sql.NullString
		var clientID sql.NullString
		if err := rows.Scan(&sess.ID, &sess.Title, &workDir, &workspaceID, &clientID, &sess.CreatedAt, &sess.UpdatedAt, &sess.AggregateVersion); err != nil {
			return nil, err
		}
		sess.WorkDir = workDir.String
		sess.WorkspaceID = workspaceID.String
		sess.ClientID = clientID.String
		out = append(out, &sess)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListSessionsByClient returns every session attributed to a specific
// Wingman API client, newest first. Sessions with no client are excluded.
func (s *SQLiteStore) ListSessionsByClient(clientID string) ([]*Session, error) {
	rows, err := s.db.Query(`
		SELECT id, title, work_dir, workspace_id, client_id, created_at, updated_at, aggregate_version FROM sessions WHERE client_id = ? ORDER BY created_at DESC
	`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Session
	for rows.Next() {
		var sess Session
		var workDir sql.NullString
		var workspaceID sql.NullString
		var cid sql.NullString
		if err := rows.Scan(&sess.ID, &sess.Title, &workDir, &workspaceID, &cid, &sess.CreatedAt, &sess.UpdatedAt, &sess.AggregateVersion); err != nil {
			return nil, err
		}
		sess.WorkDir = workDir.String
		sess.WorkspaceID = workspaceID.String
		sess.ClientID = cid.String
		out = append(out, &sess)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListSessionsByWorkspace returns every session linked to a workspace, newest first.
func (s *SQLiteStore) ListSessionsByWorkspace(workspaceID string) ([]*Session, error) {
	rows, err := s.db.Query(`
		SELECT id, title, work_dir, workspace_id, client_id, created_at, updated_at, aggregate_version FROM sessions WHERE workspace_id = ? ORDER BY created_at DESC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Session
	for rows.Next() {
		var sess Session
		var workDir sql.NullString
		var sid sql.NullString
		var cid sql.NullString
		if err := rows.Scan(&sess.ID, &sess.Title, &workDir, &sid, &cid, &sess.CreatedAt, &sess.UpdatedAt, &sess.AggregateVersion); err != nil {
			return nil, err
		}
		sess.WorkDir = workDir.String
		sess.WorkspaceID = sid.String
		sess.ClientID = cid.String
		out = append(out, &sess)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// RenameSession appends session.renamed and updates its projection atomically.
func (s *SQLiteStore) RenameSession(ctx context.Context, id, title string, expectedVersion int64) (*Session, error) {
	event, err := NewSessionRenamedEvent(id, title, Now())
	if err != nil {
		return nil, err
	}
	return s.applySessionMetadataEvent(ctx, event, expectedVersion, "")
}

// MoveSession appends session.moved and updates its projection atomically.
func (s *SQLiteStore) MoveSession(ctx context.Context, id, workDir, workspaceID string, expectedVersion int64) (*Session, error) {
	event, err := NewSessionMovedEvent(id, workDir, workspaceID, Now())
	if err != nil {
		return nil, err
	}
	return s.applySessionMetadataEvent(ctx, event, expectedVersion, workspaceID)
}

func (s *SQLiteStore) applySessionMetadataEvent(ctx context.Context, event AggregateEvent, expectedVersion int64, workspaceID string) (*Session, error) {
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	current, err := getSessionTx(ctx, tx, event.Aggregate.ID)
	if err != nil {
		return nil, err
	}
	if current.AggregateVersion != expectedVersion {
		return nil, &AggregateVersionConflict{Aggregate: event.Aggregate, Expected: expectedVersion, Actual: current.AggregateVersion}
	}
	event.ClientID = current.ClientID
	if workspaceID != "" {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM workspaces WHERE id = ?`, workspaceID).Scan(&exists); err != nil {
			if err == sql.ErrNoRows {
				return nil, fmt.Errorf("workspace not found: %s", workspaceID)
			}
			return nil, fmt.Errorf("verify workspace: %w", err)
		}
	}
	if event.Type == EventSessionRenamed {
		var data sessionRenamedData
		if err := json.Unmarshal(event.Data, &data); err != nil {
			return nil, err
		}
		if current.Title == data.Title {
			return current, nil
		}
	}
	if event.Type == EventSessionMoved {
		var data sessionMovedData
		if err := json.Unmarshal(event.Data, &data); err != nil {
			return nil, err
		}
		if current.WorkDir == data.WorkDir && current.WorkspaceID == data.WorkspaceID {
			return current, nil
		}
	}

	event, err = appendAggregateEventTx(ctx, tx, event, expectedVersion)
	if err != nil {
		return nil, err
	}
	projected, err := projectSessionEvent(current, event)
	if err != nil {
		return nil, err
	}
	var workDirPtr, workspaceIDPtr *string
	if projected.WorkDir != "" {
		workDirPtr = &projected.WorkDir
	}
	if projected.WorkspaceID != "" {
		workspaceIDPtr = &projected.WorkspaceID
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE sessions
		SET title = ?, work_dir = ?, workspace_id = ?, updated_at = ?, aggregate_version = ?
		WHERE id = ?
	`, projected.Title, workDirPtr, workspaceIDPtr, projected.UpdatedAt, projected.AggregateVersion, projected.ID); err != nil {
		return nil, fmt.Errorf("update session projection: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return projected, nil
}

func getSessionTx(ctx context.Context, tx aggregateEventTx, id string) (*Session, error) {
	var session Session
	var workDir, workspaceID, clientID sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT id, title, work_dir, workspace_id, client_id, created_at, updated_at, aggregate_version
		FROM sessions WHERE id = ?
	`, id).Scan(&session.ID, &session.Title, &workDir, &workspaceID, &clientID, &session.CreatedAt, &session.UpdatedAt, &session.AggregateVersion)
	if err == sql.ErrNoRows {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	session.WorkDir = workDir.String
	session.WorkspaceID = workspaceID.String
	session.ClientID = clientID.String
	return &session, nil
}

// PurgeSession permanently removes a session and all of its durable history.
func (s *SQLiteStore) PurgeSession(ctx context.Context, id string, expectedVersion int64) error {
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	session, err := getSessionTx(ctx, tx, id)
	if err != nil {
		return err
	}
	ref := AggregateRef{Type: AggregateSession, ID: id}
	if session.AggregateVersion != expectedVersion {
		return &AggregateVersionConflict{Aggregate: ref, Expected: expectedVersion, Actual: session.AggregateVersion}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM aggregate_events WHERE aggregate_type = ? AND aggregate_id = ?`, ref.Type, ref.ID); err != nil {
		return fmt.Errorf("delete session aggregate events: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete session projection: %w", err)
	}
	return tx.Commit(ctx)
}

// CreatePermissionRequest durably records a pending authorization request and
// its corresponding public session event in one transaction.
func (s *SQLiteStore) CreatePermissionRequest(ctx context.Context, request PermissionRequest) (PermissionRequestTransition, error) {
	if request.Status == "" {
		request.Status = PermissionRequestStatusPending
	}
	if request.Status != PermissionRequestStatusPending || request.Action == "" || len(request.Resources) == 0 {
		return PermissionRequestTransition{}, &PermissionRequestTransitionConflict{SessionID: request.SessionID, RequestID: request.ID}
	}
	for _, resource := range request.Resources {
		if resource == "" {
			return PermissionRequestTransition{}, &PermissionRequestTransitionConflict{SessionID: request.SessionID, RequestID: request.ID}
		}
	}
	if request.ID == "" {
		request.ID = NewID(PrefixPermissionRequest)
	}
	resources, err := json.Marshal(request.Resources)
	if err != nil {
		return PermissionRequestTransition{}, fmt.Errorf("marshal permission resources: %w", err)
	}
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return PermissionRequestTransition{}, err
	}
	defer tx.Rollback()
	session, err := getSessionTx(ctx, tx, request.SessionID)
	if err != nil {
		return PermissionRequestTransition{}, err
	}
	if existing, err := scanPermissionRequest(tx.QueryRowContext(ctx, `SELECT id, session_id, COALESCE(run_id, ''), COALESCE(tool_use_id, ''), call_id, action, resources_json, status, response, error_type, error_message, created_at, resolved_at, updated_at FROM permission_requests WHERE id = ?`, request.ID)); err == nil {
		if samePendingPermissionRequest(existing, request) {
			if err := tx.Commit(ctx); err != nil {
				return PermissionRequestTransition{}, err
			}
			return PermissionRequestTransition{Request: existing}, nil
		}
		return PermissionRequestTransition{}, &PermissionRequestTransitionConflict{SessionID: request.SessionID, RequestID: request.ID}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return PermissionRequestTransition{}, err
	}
	if err := validatePermissionRequestOwnershipTx(ctx, tx, request); err != nil {
		return PermissionRequestTransition{}, err
	}
	now := time.Now().UTC()
	request.CreatedAt, request.UpdatedAt = now, now
	if _, err := tx.ExecContext(ctx, `INSERT INTO permission_requests (id, session_id, run_id, tool_use_id, call_id, action, resources_json, status, response, error_type, error_message, created_at, updated_at) VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, '', '', '', ?, ?)`, request.ID, request.SessionID, request.RunID, request.ToolUseID, request.CallID, request.Action, string(resources), request.Status, formatTime(now), formatTime(now)); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: permission_requests.id") {
			return PermissionRequestTransition{}, &PermissionRequestTransitionConflict{SessionID: request.SessionID, RequestID: request.ID}
		}
		return PermissionRequestTransition{}, fmt.Errorf("insert permission request: %w", err)
	}
	event, err := appendPermissionEventTx(ctx, tx, request, "session.permission.requested", now)
	if err != nil {
		return PermissionRequestTransition{}, err
	}
	projected, err := appendPermissionRequestAggregateTx(ctx, tx, session, request, false)
	if err != nil {
		return PermissionRequestTransition{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET aggregate_version = ? WHERE id = ?`, projected.AggregateVersion, projected.ID); err != nil {
		return PermissionRequestTransition{}, fmt.Errorf("update session aggregate version: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return PermissionRequestTransition{}, err
	}
	return PermissionRequestTransition{Request: request, Event: event, Changed: true}, nil
}

func (s *SQLiteStore) GetPermissionRequest(ctx context.Context, sessionID, requestID string) (*PermissionRequest, error) {
	if err := s.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}
	request, err := scanPermissionRequest(s.db.QueryRowContext(ctx, `SELECT id, session_id, COALESCE(run_id, ''), COALESCE(tool_use_id, ''), call_id, action, resources_json, status, response, error_type, error_message, created_at, resolved_at, updated_at FROM permission_requests WHERE id = ? AND session_id = ?`, requestID, sessionID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &PermissionRequestNotFound{SessionID: sessionID, RequestID: requestID}
	}
	if err != nil {
		return nil, err
	}
	return &request, nil
}

func (s *SQLiteStore) ListPermissionRequests(ctx context.Context, sessionID string) ([]PermissionRequest, error) {
	if err := s.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, session_id, COALESCE(run_id, ''), COALESCE(tool_use_id, ''), call_id, action, resources_json, status, response, error_type, error_message, created_at, resolved_at, updated_at FROM permission_requests WHERE session_id = ? ORDER BY created_at, id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PermissionRequest{}
	for rows.Next() {
		request, err := scanPermissionRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, request)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ResolvePermissionRequest(ctx context.Context, resolution PermissionRequestResolution) (PermissionRequestTransition, error) {
	if resolution.ExpectedStatus == "" {
		resolution.ExpectedStatus = PermissionRequestStatusPending
	}
	if resolution.ExpectedStatus != PermissionRequestStatusPending || !legalPermissionResolution(resolution.Status, resolution.Response) {
		return PermissionRequestTransition{}, &PermissionRequestTransitionConflict{SessionID: resolution.SessionID, RequestID: resolution.RequestID}
	}
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return PermissionRequestTransition{}, err
	}
	defer tx.Rollback()
	session, err := getSessionTx(ctx, tx, resolution.SessionID)
	if err != nil {
		return PermissionRequestTransition{}, err
	}
	request, err := scanPermissionRequest(tx.QueryRowContext(ctx, `SELECT id, session_id, COALESCE(run_id, ''), COALESCE(tool_use_id, ''), call_id, action, resources_json, status, response, error_type, error_message, created_at, resolved_at, updated_at FROM permission_requests WHERE id = ? AND session_id = ?`, resolution.RequestID, resolution.SessionID))
	if errors.Is(err, sql.ErrNoRows) {
		return PermissionRequestTransition{}, &PermissionRequestNotFound{SessionID: resolution.SessionID, RequestID: resolution.RequestID}
	}
	if err != nil {
		return PermissionRequestTransition{}, err
	}
	if request.Status != PermissionRequestStatusPending {
		if samePermissionResolution(request, resolution) {
			return PermissionRequestTransition{Request: request}, tx.Commit(ctx)
		}
		return PermissionRequestTransition{}, &PermissionRequestTransitionConflict{SessionID: resolution.SessionID, RequestID: resolution.RequestID}
	}
	now := time.Now().UTC()
	request.Status, request.Response, request.ErrorType, request.ErrorMessage = resolution.Status, resolution.Response, resolution.ErrorType, resolution.ErrorMessage
	request.ResolvedAt, request.UpdatedAt = now, now
	if _, err := tx.ExecContext(ctx, `UPDATE permission_requests SET status = ?, response = ?, error_type = ?, error_message = ?, resolved_at = ?, updated_at = ? WHERE id = ? AND session_id = ? AND status = ?`, request.Status, request.Response, request.ErrorType, request.ErrorMessage, formatTime(now), formatTime(now), request.ID, request.SessionID, PermissionRequestStatusPending); err != nil {
		return PermissionRequestTransition{}, err
	}
	event, err := appendPermissionEventTx(ctx, tx, request, "session.permission.resolved", now)
	if err != nil {
		return PermissionRequestTransition{}, err
	}
	projected, err := appendPermissionRequestAggregateTx(ctx, tx, session, request, true)
	if err != nil {
		return PermissionRequestTransition{}, err
	}
	if resolution.Response == PermissionResponseAlways {
		for _, resource := range request.Resources {
			grant := PermissionGrant{ID: NewID(PrefixPermissionGrant), SessionID: request.SessionID, Action: request.Action, Resource: resource, CreatedAt: now}
			result, err := tx.ExecContext(ctx, `INSERT INTO permission_grants (id, session_id, action, resource, created_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT(session_id, action, resource) DO NOTHING`, grant.ID, grant.SessionID, grant.Action, grant.Resource, formatTime(now))
			if err != nil {
				return PermissionRequestTransition{}, fmt.Errorf("insert permission grant: %w", err)
			}
			if changed, err := result.RowsAffected(); err != nil {
				return PermissionRequestTransition{}, fmt.Errorf("read permission grant insert result: %w", err)
			} else if changed != 0 {
				projected, err = appendPermissionGrantAggregateTx(ctx, tx, projected, grant)
				if err != nil {
					return PermissionRequestTransition{}, err
				}
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET aggregate_version = ? WHERE id = ?`, projected.AggregateVersion, projected.ID); err != nil {
		return PermissionRequestTransition{}, fmt.Errorf("update session aggregate version: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return PermissionRequestTransition{}, err
	}
	return PermissionRequestTransition{Request: request, Event: event, Changed: true}, nil
}

func (s *SQLiteStore) ListPermissionGrants(ctx context.Context, sessionID string) ([]PermissionGrant, error) {
	if err := s.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, session_id, action, resource, created_at FROM permission_grants WHERE session_id = ? ORDER BY created_at, id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PermissionGrant{}
	for rows.Next() {
		var grant PermissionGrant
		var created string
		if err := rows.Scan(&grant.ID, &grant.SessionID, &grant.Action, &grant.Resource, &created); err != nil {
			return nil, err
		}
		grant.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, grant)
	}
	return out, rows.Err()
}

// InterruptPendingPermissionRequests resolves requests left pending at startup.
func (s *SQLiteStore) InterruptPendingPermissionRequests(ctx context.Context) ([]PermissionRequestTransition, error) {
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id, session_id, COALESCE(run_id, ''), COALESCE(tool_use_id, ''), call_id, action, resources_json, status, response, error_type, error_message, created_at, resolved_at, updated_at FROM permission_requests WHERE status = ? ORDER BY created_at, id`, PermissionRequestStatusPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	requests := []PermissionRequest{}
	for rows.Next() {
		request, err := scanPermissionRequest(rows)
		if err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	transitions := make([]PermissionRequestTransition, 0, len(requests))
	for _, request := range requests {
		request.Status, request.ErrorType, request.ErrorMessage = PermissionRequestStatusInterrupted, "process_interrupted", "permission request interrupted because the process stopped"
		request.ResolvedAt, request.UpdatedAt = now, now
		if _, err := tx.ExecContext(ctx, `UPDATE permission_requests SET status = ?, error_type = ?, error_message = ?, resolved_at = ?, updated_at = ? WHERE id = ? AND status = ?`, request.Status, request.ErrorType, request.ErrorMessage, formatTime(now), formatTime(now), request.ID, PermissionRequestStatusPending); err != nil {
			return nil, err
		}
		session, err := getSessionTx(ctx, tx, request.SessionID)
		if err != nil {
			return nil, err
		}
		event, err := appendPermissionEventTx(ctx, tx, request, "session.permission.resolved", now)
		if err != nil {
			return nil, err
		}
		projected, err := appendPermissionRequestAggregateTx(ctx, tx, session, request, true)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE sessions SET aggregate_version = ? WHERE id = ?`, projected.AggregateVersion, projected.ID); err != nil {
			return nil, fmt.Errorf("update session aggregate version: %w", err)
		}
		transitions = append(transitions, PermissionRequestTransition{Request: request, Event: event, Changed: true})
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return transitions, nil
}

// SaveMessage atomically stores a complete authoritative message revision.
func (s *SQLiteStore) SaveMessage(ctx context.Context, msg StoredMessage) error {
	if msg.Revision == 0 {
		msg.Revision = 1
	}
	if msg.State == "" {
		msg.State = "completed"
	}
	if err := validateMessageParts(msg); err != nil {
		return err
	}
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	session, err := getSessionTx(ctx, tx, msg.SessionID)
	if err != nil {
		return err
	}
	var existing StoredMessage
	var metadata sql.NullString
	var created, updated string
	err = tx.QueryRowContext(ctx, `SELECT id, session_id, COALESCE(run_id, ''), idx, role, revision, state, metadata_json, created_at, updated_at FROM messages WHERE id = ?`, msg.ID).Scan(&existing.ID, &existing.SessionID, &existing.RunID, &existing.Idx, &existing.Role, &existing.Revision, &existing.State, &metadata, &created, &updated)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read message: %w", err)
	}
	if errors.Is(err, sql.ErrNoRows) {
		var conflicting string
		err = tx.QueryRowContext(ctx, `SELECT id FROM messages WHERE session_id = ? AND idx = ?`, msg.SessionID, msg.Idx).Scan(&conflicting)
		if err == nil {
			return fmt.Errorf("message index belongs to %s", conflicting)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := insertMessageTx(ctx, tx, msg, time.Now().UTC()); err != nil {
			return err
		}
	} else {
		if existing.SessionID != msg.SessionID || existing.RunID != msg.RunID || existing.Idx != msg.Idx || existing.Role != msg.Role {
			return fmt.Errorf("message identity is immutable")
		}
		existing.MetadataJSON = nullableStringBytes(metadata)
		existing.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		existing.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		existing.Parts, err = listMessagePartsTx(ctx, tx, existing.ID)
		if err != nil {
			return err
		}
		if msg.Revision < existing.Revision {
			return ErrMessageRevisionStale
		}
		if msg.Revision == existing.Revision {
			if messageRevisionEqual(existing, msg) {
				return tx.Commit(ctx)
			}
			return ErrMessageRevisionConflict
		}
		if err := replaceMessageRevisionTx(ctx, tx, existing, msg, time.Now().UTC()); err != nil {
			return err
		}
	}
	persisted, err := getStoredMessageTx(ctx, tx, msg.ID)
	if err != nil {
		return err
	}
	aggregateEvent, err := NewSessionMessageSavedEvent(persisted)
	if err != nil {
		return err
	}
	aggregateEvent, err = appendAggregateEventTx(ctx, tx, aggregateEvent, session.AggregateVersion)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET aggregate_version = ? WHERE id = ?`, aggregateEvent.Version, msg.SessionID); err != nil {
		return fmt.Errorf("update session message version: %w", err)
	}
	return tx.Commit(ctx)
}

func validateMessageParts(msg StoredMessage) error {
	partIDs := make(map[string]struct{}, len(msg.Parts))
	sequences := make(map[int]struct{}, len(msg.Parts))
	for _, part := range msg.Parts {
		if part.MessageID != msg.ID {
			return fmt.Errorf("part %s does not belong to message %s", part.ID, msg.ID)
		}
		if _, ok := partIDs[part.ID]; ok {
			return fmt.Errorf("duplicate part ID %s", part.ID)
		}
		if _, ok := sequences[part.Sequence]; ok {
			return fmt.Errorf("duplicate part sequence %d", part.Sequence)
		}
		partIDs[part.ID] = struct{}{}
		sequences[part.Sequence] = struct{}{}
	}
	return nil
}

func nullableStringBytes(v sql.NullString) []byte {
	if !v.Valid {
		return nil
	}
	return []byte(v.String)
}

func messageRevisionEqual(existing, incoming StoredMessage) bool {
	if existing.Role != incoming.Role || existing.State != incoming.State || !bytes.Equal(existing.MetadataJSON, incoming.MetadataJSON) || len(existing.Parts) != len(incoming.Parts) {
		return false
	}
	for i := range existing.Parts {
		a, b := existing.Parts[i], incoming.Parts[i]
		if a.ID != b.ID || a.Sequence != b.Sequence || a.Kind != b.Kind || !bytes.Equal(a.PayloadJSON, b.PayloadJSON) {
			return false
		}
	}
	return true
}

func insertMessageTx(ctx context.Context, tx *immediateTx, msg StoredMessage, now time.Time) error {
	created, updated := messageTimes(msg, now)
	if msg.RunID != "" {
		var runSession string
		if err := tx.QueryRowContext(ctx, `SELECT session_id FROM session_runs WHERE id = ?`, msg.RunID).Scan(&runSession); err != nil {
			return fmt.Errorf("message run ownership: %w", err)
		}
		if runSession != msg.SessionID {
			return fmt.Errorf("session run %s does not belong to session %s", msg.RunID, msg.SessionID)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO messages (id, session_id, run_id, idx, role, revision, state, metadata_json, created_at, updated_at) VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?)`, msg.ID, msg.SessionID, msg.RunID, msg.Idx, msg.Role, msg.Revision, msg.State, nullableJSON(msg.MetadataJSON), created, updated); err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	if err := insertPartsTx(ctx, tx, msg.Parts, nil, now); err != nil {
		return err
	}
	return nil
}

func replaceMessageRevisionTx(ctx context.Context, tx *immediateTx, existing, msg StoredMessage, now time.Time) error {
	_, updated := messageTimes(msg, now)
	if _, err := tx.ExecContext(ctx, `UPDATE messages SET revision = ?, state = ?, metadata_json = ?, updated_at = ? WHERE id = ?`, msg.Revision, msg.State, nullableJSON(msg.MetadataJSON), updated, msg.ID); err != nil {
		return fmt.Errorf("update message: %w", err)
	}
	oldParts := make(map[string]StoredPart, len(existing.Parts))
	for _, part := range existing.Parts {
		oldParts[part.ID] = part
	}
	for _, part := range msg.Parts {
		var owner string
		err := tx.QueryRowContext(ctx, `SELECT message_id FROM parts WHERE id = ?`, part.ID).Scan(&owner)
		if err == nil && owner != msg.ID {
			return fmt.Errorf("part %s belongs to message %s", part.ID, owner)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	newParts := make(map[string]struct{}, len(msg.Parts))
	for _, part := range msg.Parts {
		newParts[part.ID] = struct{}{}
	}
	for _, part := range existing.Parts {
		if _, ok := newParts[part.ID]; ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM parts WHERE id = ?`, part.ID); err != nil {
			return fmt.Errorf("delete message part: %w", err)
		}
	}
	if err := upsertPartsTx(ctx, tx, msg.Parts, oldParts, now); err != nil {
		return err
	}
	return nil
}

func getStoredMessageTx(ctx context.Context, tx *immediateTx, messageID string) (StoredMessage, error) {
	var message StoredMessage
	var metadata sql.NullString
	var created, updated string
	if err := tx.QueryRowContext(ctx, `SELECT id, session_id, COALESCE(run_id, ''), idx, role, revision, state, metadata_json, created_at, updated_at FROM messages WHERE id = ?`, messageID).Scan(&message.ID, &message.SessionID, &message.RunID, &message.Idx, &message.Role, &message.Revision, &message.State, &metadata, &created, &updated); err != nil {
		return StoredMessage{}, fmt.Errorf("read stored message: %w", err)
	}
	message.MetadataJSON = nullableStringBytes(metadata)
	message.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	message.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	var err error
	message.Parts, err = listMessagePartsTx(ctx, tx, message.ID)
	return message, err
}

func messageTimes(msg StoredMessage, now time.Time) (string, string) {
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = now
	}
	if msg.UpdatedAt.IsZero() {
		msg.UpdatedAt = now
	}
	return msg.CreatedAt.UTC().Format(time.RFC3339Nano), msg.UpdatedAt.UTC().Format(time.RFC3339Nano)
}

func insertPartsTx(ctx context.Context, tx *immediateTx, parts []StoredPart, old map[string]StoredPart, now time.Time) error {
	return writePartsTx(ctx, tx, parts, old, now, false)
}

func upsertPartsTx(ctx context.Context, tx *immediateTx, parts []StoredPart, old map[string]StoredPart, now time.Time) error {
	return writePartsTx(ctx, tx, parts, old, now, true)
}

func writePartsTx(ctx context.Context, tx *immediateTx, parts []StoredPart, old map[string]StoredPart, now time.Time, replace bool) error {
	for _, part := range parts {
		if oldPart, ok := old[part.ID]; ok {
			part.CreatedAt = oldPart.CreatedAt
		} else if part.CreatedAt.IsZero() {
			part.CreatedAt = now
		}
		if part.UpdatedAt.IsZero() {
			part.UpdatedAt = now
		}
		query := `INSERT INTO parts (id, message_id, idx, kind, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
		if replace {
			query += ` ON CONFLICT(id) DO UPDATE SET message_id = excluded.message_id, idx = excluded.idx, kind = excluded.kind, payload_json = excluded.payload_json, updated_at = excluded.updated_at`
		}
		if _, err := tx.ExecContext(ctx, query, part.ID, part.MessageID, part.Sequence, part.Kind, string(part.PayloadJSON), part.CreatedAt.UTC().Format(time.RFC3339Nano), part.UpdatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert message part: %w", err)
		}
	}
	return nil
}

func listMessagePartsTx(ctx context.Context, tx *immediateTx, messageID string) ([]StoredPart, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, message_id, idx, kind, payload_json, created_at, updated_at FROM parts WHERE message_id = ? ORDER BY idx ASC`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	parts := make([]StoredPart, 0)
	for rows.Next() {
		var part StoredPart
		var payload, created, updated string
		if err := rows.Scan(&part.ID, &part.MessageID, &part.Sequence, &part.Kind, &payload, &created, &updated); err != nil {
			return nil, err
		}
		part.PayloadJSON = []byte(payload)
		part.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		part.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		parts = append(parts, part)
	}
	return parts, rows.Err()
}

// ListMessages returns all messages for the session ordered by Idx ASC,
// with each message's Parts populated and ordered by Sequence (idx) ASC.
// Returns ErrSessionNotFound if the session does not exist.
// Returns an empty slice (not nil) when the session has no messages.
func (s *SQLiteStore) ListMessages(ctx context.Context, sessionID string) ([]StoredMessage, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM sessions WHERE id = ?`, sessionID).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT id, session_id, COALESCE(run_id, ''), idx, role, revision, state, metadata_json, created_at, updated_at
		FROM messages
		WHERE session_id = ?
		ORDER BY idx ASC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var msgs []StoredMessage
	for rows.Next() {
		var m StoredMessage
		var metadataJSON sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&m.ID, &m.SessionID, &m.RunID, &m.Idx, &m.Role, &m.Revision, &m.State, &metadataJSON, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		m.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		if metadataJSON.Valid {
			m.MetadataJSON = []byte(metadataJSON.String)
		} else {
			m.MetadataJSON = nil
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(msgs) == 0 {
		return []StoredMessage{}, nil
	}

	partRows, err := tx.QueryContext(ctx, `
		SELECT p.id, p.message_id, p.idx, p.kind, p.payload_json, p.created_at, p.updated_at
		FROM parts p
		JOIN messages m ON p.message_id = m.id
		WHERE m.session_id = ?
		ORDER BY p.message_id, p.idx ASC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query parts: %w", err)
	}
	defer partRows.Close()

	msgMap := make(map[string]*StoredMessage, len(msgs))
	for i := range msgs {
		msgMap[msgs[i].ID] = &msgs[i]
	}

	for partRows.Next() {
		var p StoredPart
		var payload, createdAt, updatedAt string
		if err := partRows.Scan(&p.ID, &p.MessageID, &p.Sequence, &p.Kind, &payload, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		p.PayloadJSON = []byte(payload)
		p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		if m, ok := msgMap[p.MessageID]; ok {
			m.Parts = append(m.Parts, p)
		}
	}
	if err := partRows.Err(); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return msgs, nil
}

// UpsertModelCall inserts or updates one upstream model-call record.
func (s *SQLiteStore) UpsertModelCall(ctx context.Context, call ModelCall) error {
	if call.ID == "" {
		call.ID = NewID(PrefixModelCall)
	}
	if call.Attempt == 0 {
		call.Attempt = 1
	}
	now := time.Now().UTC()
	if call.StartedAt.IsZero() {
		call.StartedAt = now
	}
	if call.CreatedAt.IsZero() {
		call.CreatedAt = now
	}
	if call.UpdatedAt.IsZero() {
		call.UpdatedAt = now
	}
	startedAt := call.StartedAt.UTC().Format(time.RFC3339Nano)
	var completedAt *string
	if !call.CompletedAt.IsZero() {
		v := call.CompletedAt.UTC().Format(time.RFC3339Nano)
		completedAt = &v
	}
	createdAt := call.CreatedAt.UTC().Format(time.RFC3339Nano)
	updatedAt := call.UpdatedAt.UTC().Format(time.RFC3339Nano)
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	existing, err := scanModelCall(tx.QueryRowContext(ctx, `SELECT `+modelCallColumns+` FROM model_calls WHERE id = ?`, call.ID))
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read model call: %w", err)
	}
	if err == nil {
		call.SessionID, call.RunID, call.Step, call.Attempt = existing.SessionID, existing.RunID, existing.Step, existing.Attempt
		call.StartedAt, call.CreatedAt = existing.StartedAt, existing.CreatedAt
		startedAt, createdAt = call.StartedAt.UTC().Format(time.RFC3339Nano), call.CreatedAt.UTC().Format(time.RFC3339Nano)
	} else if call.RunID != "" {
		var runExists int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM session_runs WHERE id = ? AND session_id = ?`, call.RunID, call.SessionID).Scan(&runExists); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("session run %s does not belong to session %s", call.RunID, call.SessionID)
			}
			return fmt.Errorf("check model call session run: %w", err)
		}
		var existingID string
		err := tx.QueryRowContext(ctx, `
			SELECT id FROM model_calls
			WHERE run_id = ? AND step = ? AND attempt = ?
		`, call.RunID, call.Step, call.Attempt).Scan(&existingID)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("check model call attempt: %w", err)
		}
		if err == nil && existingID != call.ID {
			return ErrModelCallAttemptConflict
		}
	}
	session, err := getSessionTx(ctx, tx, call.SessionID)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO model_calls (
			id, session_id, run_id, assistant_message_id, step, attempt, status,
			agent_id, model_ref, provider, provider_request_id, api, model_id,
			finish_reason, stop_reason, error_type, error_message,
			input_tokens, output_tokens, reasoning_tokens, cached_input_tokens, cache_write_tokens, total_tokens,
			context_tokens, context_window, context_percent, cost,
			structured_output_json, metadata_json, started_at, completed_at, created_at, updated_at
		)
		VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			assistant_message_id = excluded.assistant_message_id,
			status = excluded.status,
			agent_id = excluded.agent_id,
			model_ref = excluded.model_ref,
			provider = excluded.provider,
			provider_request_id = excluded.provider_request_id,
			api = excluded.api,
			model_id = excluded.model_id,
			finish_reason = excluded.finish_reason,
			stop_reason = excluded.stop_reason,
			error_type = excluded.error_type,
			error_message = excluded.error_message,
			input_tokens = excluded.input_tokens,
			output_tokens = excluded.output_tokens,
			reasoning_tokens = excluded.reasoning_tokens,
			cached_input_tokens = excluded.cached_input_tokens,
			cache_write_tokens = excluded.cache_write_tokens,
			total_tokens = excluded.total_tokens,
			context_tokens = excluded.context_tokens,
			context_window = excluded.context_window,
			context_percent = excluded.context_percent,
			cost = excluded.cost,
			structured_output_json = excluded.structured_output_json,
			metadata_json = excluded.metadata_json,
			completed_at = excluded.completed_at,
			updated_at = excluded.updated_at
	`, call.ID, call.SessionID, call.RunID, call.AssistantMessageID, call.Step, call.Attempt, call.Status,
		call.AgentID, call.ModelRef, call.Provider, call.ProviderRequestID, call.API, call.ModelID,
		call.FinishReason, call.StopReason, call.ErrorType, call.ErrorMessage,
		call.InputTokens, call.OutputTokens, call.ReasoningTokens, call.CachedInputTokens, call.CacheWriteTokens, call.TotalTokens,
		call.ContextTokens, call.ContextWindow, call.ContextPercent, call.Cost,
		nullableBytes(call.StructuredOutputJSON), nullableBytes(call.MetadataJSON), startedAt, completedAt, createdAt, updatedAt)
	if err != nil {
		if call.RunID != "" {
			var conflictingID string
			conflictErr := tx.QueryRowContext(ctx, `SELECT id FROM model_calls WHERE run_id = ? AND step = ? AND attempt = ?`, call.RunID, call.Step, call.Attempt).Scan(&conflictingID)
			if conflictErr == nil && conflictingID != call.ID {
				return ErrModelCallAttemptConflict
			}
		}
		return fmt.Errorf("upsert model call: %w", err)
	}
	event, err := NewSessionModelCallSavedEvent(call)
	if err != nil {
		return err
	}
	event, err = appendAggregateEventTx(ctx, tx, event, session.AggregateVersion)
	if err != nil {
		return err
	}
	projected, err := projectSessionEvent(session, event)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET aggregate_version = ? WHERE id = ?`, projected.AggregateVersion, projected.ID); err != nil {
		return fmt.Errorf("update session aggregate version: %w", err)
	}
	return tx.Commit(ctx)
}

// LatestModelCall returns the latest call with context usage for a session.
func (s *SQLiteStore) LatestModelCall(ctx context.Context, sessionID string) (*ModelCall, error) {
	if err := s.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT `+modelCallColumns+`
		FROM model_calls
		WHERE session_id = ? AND context_tokens > 0
		ORDER BY started_at DESC, id DESC
		LIMIT 1
	`, sessionID)
	call, err := scanModelCall(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &call, nil
}

// ListModelCalls returns all model calls for the session in chronological order.
func (s *SQLiteStore) ListModelCalls(ctx context.Context, sessionID string) ([]ModelCall, error) {
	if err := s.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+modelCallColumns+`
		FROM model_calls
		WHERE session_id = ?
		ORDER BY started_at ASC, id ASC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query model calls: %w", err)
	}
	defer rows.Close()

	var out []ModelCall
	for rows.Next() {
		call, err := scanModelCall(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, call)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []ModelCall{}
	}
	return out, nil
}

// InterruptActiveModelCalls aborts started calls owned by a run. Terminal calls are unchanged.
func (s *SQLiteStore) InterruptActiveModelCalls(ctx context.Context, runID, errorType, errorMessage string) error {
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT `+modelCallColumns+` FROM model_calls WHERE run_id = ? AND status = ?`, runID, ModelCallStatusStarted)
	if err != nil {
		return fmt.Errorf("list active model calls: %w", err)
	}
	defer rows.Close()
	var calls []ModelCall
	for rows.Next() {
		call, err := scanModelCall(rows)
		if err != nil {
			return err
		}
		calls = append(calls, call)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, call := range calls {
		call.Status, call.ErrorType, call.ErrorMessage = ModelCallStatusAborted, errorType, errorMessage
		call.CompletedAt, call.UpdatedAt = now, now
		if _, err := tx.ExecContext(ctx, `UPDATE model_calls SET status = ?, error_type = ?, error_message = ?, completed_at = ?, updated_at = ? WHERE id = ?`, call.Status, call.ErrorType, call.ErrorMessage, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), call.ID); err != nil {
			return fmt.Errorf("interrupt active model calls: %w", err)
		}
		session, err := getSessionTx(ctx, tx, call.SessionID)
		if err != nil {
			return err
		}
		event, err := NewSessionModelCallSavedEvent(call)
		if err != nil {
			return err
		}
		event, err = appendAggregateEventTx(ctx, tx, event, session.AggregateVersion)
		if err != nil {
			return err
		}
		projected, err := projectSessionEvent(session, event)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE sessions SET aggregate_version = ? WHERE id = ?`, projected.AggregateVersion, projected.ID); err != nil {
			return fmt.Errorf("update session aggregate version: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// SaveToolUse inserts or authoritatively advances one tool-use lifecycle.
func (s *SQLiteStore) SaveToolUse(ctx context.Context, use ToolUse) error {
	if use.ID == "" {
		use.ID = NewID(PrefixToolUse)
	}
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := toolUseSessionExists(ctx, tx, use.SessionID); err != nil {
		return err
	}
	existing, err := scanToolUse(tx.QueryRowContext(ctx, `SELECT `+toolUseColumns+` FROM tool_uses WHERE id = ?`, use.ID))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read tool use: %w", err)
	}
	now := time.Now().UTC()
	if errors.Is(err, sql.ErrNoRows) {
		if use.Status != ToolUseStatusProposed {
			return ErrToolUseInvalidTransition
		}
		if use.RunID != "" {
			var runSession string
			if err := tx.QueryRowContext(ctx, `SELECT session_id FROM session_runs WHERE id = ?`, use.RunID).Scan(&runSession); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return fmt.Errorf("session run %s does not belong to session %s", use.RunID, use.SessionID)
				}
				return err
			}
			if runSession != use.SessionID {
				return fmt.Errorf("session run %s does not belong to session %s", use.RunID, use.SessionID)
			}
			var conflictingID string
			err := tx.QueryRowContext(ctx, `SELECT id FROM tool_uses WHERE run_id = ? AND step = ? AND ordinal = ?`, use.RunID, use.Step, use.Ordinal).Scan(&conflictingID)
			if err == nil && conflictingID != use.ID {
				return ErrToolUseIdentityConflict
			}
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		if use.ProposedAt.IsZero() {
			use.ProposedAt = now
		}
		if use.CreatedAt.IsZero() {
			use.CreatedAt = now
		}
		if use.UpdatedAt.IsZero() {
			use.UpdatedAt = now
		}
		if err := insertToolUse(ctx, tx, use); err != nil {
			return fmt.Errorf("insert tool use: %w", err)
		}
		if err := appendToolUseAggregateTx(ctx, tx, use); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if !sameToolUseIdentity(existing, use) {
		return ErrToolUseIdentityConflict
	}
	use.CreatedAt = existing.CreatedAt
	use.ProposedAt = existing.ProposedAt
	if use.AuthorizedAt.IsZero() {
		use.AuthorizedAt = existing.AuthorizedAt
	}
	if use.StartedAt.IsZero() {
		use.StartedAt = existing.StartedAt
	}
	if use.CompletedAt.IsZero() {
		use.CompletedAt = existing.CompletedAt
	}
	if use.UpdatedAt.IsZero() {
		use.UpdatedAt = existing.UpdatedAt
	}
	if use.Status == existing.Status {
		if !sameToolUse(existing, use) {
			return ErrToolUseInvalidTransition
		}
		return tx.Commit(ctx)
	}
	if !legalToolUseTransition(existing.Status, use.Status) {
		return ErrToolUseInvalidTransition
	}
	if use.Status != ToolUseStatusAuthorized && !bytes.Equal(existing.InputJSON, use.InputJSON) {
		return ErrToolUseInvalidTransition
	}
	if use.UpdatedAt.IsZero() || use.UpdatedAt.Equal(existing.UpdatedAt) {
		use.UpdatedAt = now
	}
	if use.Status == ToolUseStatusAuthorized && use.AuthorizedAt.IsZero() {
		use.AuthorizedAt = now
	}
	if use.Status == ToolUseStatusStarted && use.StartedAt.IsZero() {
		use.StartedAt = now
	}
	if isToolUseTerminal(use.Status) && use.CompletedAt.IsZero() {
		use.CompletedAt = now
	}
	if err := updateToolUse(ctx, tx, use); err != nil {
		return fmt.Errorf("update tool use: %w", err)
	}
	if err := appendToolUseAggregateTx(ctx, tx, use); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func appendToolUseAggregateTx(ctx context.Context, tx *immediateTx, use ToolUse) error {
	session, err := getSessionTx(ctx, tx, use.SessionID)
	if err != nil {
		return err
	}
	event, err := NewSessionToolUseSavedEvent(use)
	if err != nil {
		return err
	}
	event, err = appendAggregateEventTx(ctx, tx, event, session.AggregateVersion)
	if err != nil {
		return err
	}
	projected, err := projectSessionEvent(session, event)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET aggregate_version = ? WHERE id = ?`, projected.AggregateVersion, projected.ID); err != nil {
		return fmt.Errorf("update session tool use aggregate version: %w", err)
	}
	return nil
}

// ListToolUses returns tool uses in their source order for a session.
func (s *SQLiteStore) ListToolUses(ctx context.Context, sessionID string) ([]ToolUse, error) {
	if err := s.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+toolUseColumns+` FROM tool_uses WHERE session_id = ? ORDER BY proposed_at, step, ordinal, id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query tool uses: %w", err)
	}
	defer rows.Close()
	out := []ToolUse{}
	for rows.Next() {
		use, err := scanToolUse(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, use)
	}
	return out, rows.Err()
}

// InterruptActiveToolUses marks work that cannot survive process shutdown.
func (s *SQLiteStore) InterruptActiveToolUses(ctx context.Context) error {
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT `+toolUseColumns+` FROM tool_uses WHERE status IN (?, ?, ?) ORDER BY proposed_at, step, ordinal, id`, ToolUseStatusProposed, ToolUseStatusAuthorized, ToolUseStatusStarted)
	if err != nil {
		return fmt.Errorf("list active tool uses: %w", err)
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
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, use := range uses {
		use.Status, use.ErrorType, use.ErrorMessage = ToolUseStatusInterrupted, "process_interrupted", "tool use interrupted because the process stopped"
		use.CompletedAt, use.UpdatedAt = now, now
		if err := updateToolUse(ctx, tx, use); err != nil {
			return fmt.Errorf("interrupt active tool uses: %w", err)
		}
		if err := appendToolUseAggregateTx(ctx, tx, use); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *SQLiteStore) AdmitSessionRun(ctx context.Context, run SessionRun) (SessionRunAdmission, error) {
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return SessionRunAdmission{}, err
	}
	defer tx.Rollback()

	session, err := getSessionTx(ctx, tx, run.SessionID)
	if err != nil {
		return SessionRunAdmission{}, err
	}
	run.WorkDir, run.WorkspaceID, run.ClientID = session.WorkDir, session.WorkspaceID, session.ClientID
	hash, err := SessionRunRequestHash(run)
	if err != nil {
		return SessionRunAdmission{}, err
	}
	if run.RequestID != "" {
		existing, err := scanSessionRun(tx.QueryRowContext(ctx, `SELECT `+sessionRunColumns+` FROM session_runs WHERE session_id = ? AND request_id = ?`, run.SessionID, run.RequestID))
		if err == nil {
			if existing.RequestHash != hash {
				return SessionRunAdmission{}, &SessionRunAdmissionConflict{SessionID: run.SessionID, RequestID: run.RequestID}
			}
			return SessionRunAdmission{Run: existing, SessionVersion: session.AggregateVersion}, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return SessionRunAdmission{}, err
		}
	}
	if run.ID == "" {
		run.ID = NewID(PrefixRun)
	}
	agentJSON, err := json.Marshal(run.Agent)
	if err != nil {
		return SessionRunAdmission{}, fmt.Errorf("marshal run agent: %w", err)
	}
	var next int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence), 0) + 1 FROM session_runs WHERE session_id = ?`, run.SessionID).Scan(&next); err != nil {
		return SessionRunAdmission{}, err
	}
	now := time.Now().UTC()
	run.Sequence, run.Status, run.RequestHash = next, SessionRunStatusQueued, hash
	run.AdmittedVersion = session.AggregateVersion + 1
	run.CreatedAt, run.UpdatedAt = now, now
	aggregateEvent, err := NewSessionRunAdmittedEvent(run)
	if err != nil {
		return SessionRunAdmission{}, err
	}
	if _, err = appendAggregateEventTx(ctx, tx, aggregateEvent, session.AggregateVersion); err != nil {
		return SessionRunAdmission{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO session_runs (id, session_id, request_id, request_hash, admitted_version, work_dir, workspace_id, client_id, sequence, status, message, agent_json, output_schema_json, error_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, NULL, ?, ?)
	`, run.ID, run.SessionID, run.RequestID, run.RequestHash, run.AdmittedVersion, run.WorkDir, run.WorkspaceID, run.ClientID, run.Sequence, run.Status, run.Message, string(agentJSON), nullableJSON(run.OutputSchemaJSON), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		return SessionRunAdmission{}, fmt.Errorf("insert session run: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET aggregate_version = ? WHERE id = ?`, run.AdmittedVersion, run.SessionID); err != nil {
		return SessionRunAdmission{}, fmt.Errorf("update session admission version: %w", err)
	}
	queuedData, err := json.Marshal(struct {
		RunID   string `json:"run_id"`
		Message string `json:"message"`
	}{run.ID, run.Message})
	if err != nil {
		return SessionRunAdmission{}, err
	}
	var maxSeq sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(seq) FROM session_events WHERE session_id = ?`, run.SessionID).Scan(&maxSeq); err != nil {
		return SessionRunAdmission{}, err
	}
	queued := SessionEvent{ID: NewID(PrefixEvent), SchemaVersion: 1, Type: "session.run.queued", Time: now, SessionID: run.SessionID, Seq: maxSeq.Int64 + 1, DataJSON: queuedData, Data: queuedData}
	if _, err := tx.ExecContext(ctx, `INSERT INTO session_events (id, session_id, seq, schema_version, type, data_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, queued.ID, queued.SessionID, queued.Seq, queued.SchemaVersion, queued.Type, string(queued.DataJSON), now.Format(time.RFC3339Nano)); err != nil {
		return SessionRunAdmission{}, fmt.Errorf("insert queued session event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return SessionRunAdmission{}, err
	}
	return SessionRunAdmission{Run: run, SessionVersion: run.AdmittedVersion, Created: true, QueuedEvent: queued}, nil
}

func (s *SQLiteStore) GetSessionRun(ctx context.Context, sessionID, runID string) (*SessionRun, error) {
	if err := s.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}
	run, err := scanSessionRun(s.db.QueryRowContext(ctx, `SELECT `+sessionRunColumns+` FROM session_runs WHERE id = ? AND session_id = ?`, runID, sessionID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionRunNotFound
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *SQLiteStore) ListSessionRuns(ctx context.Context, sessionID string) ([]SessionRun, error) {
	if err := s.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+sessionRunColumns+` FROM session_runs WHERE session_id = ? ORDER BY sequence`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionRun{}
	for rows.Next() {
		run, err := scanSessionRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ClaimNextSessionRun(ctx context.Context, sessionID string) (SessionRunTransition, error) {
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return SessionRunTransition{}, err
	}
	defer tx.Rollback()
	session, err := getSessionTx(ctx, tx, sessionID)
	if err != nil {
		return SessionRunTransition{}, err
	}
	var running int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM session_runs WHERE session_id = ? AND status = ?`, sessionID, SessionRunStatusRunning).Scan(&running); err != nil {
		return SessionRunTransition{}, err
	}
	if running != 0 {
		if err := tx.Commit(ctx); err != nil {
			return SessionRunTransition{}, err
		}
		return SessionRunTransition{}, nil
	}
	run, err := scanSessionRun(tx.QueryRowContext(ctx, `SELECT `+sessionRunColumns+` FROM session_runs WHERE session_id = ? AND status = ? ORDER BY sequence LIMIT 1`, sessionID, SessionRunStatusQueued))
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return SessionRunTransition{}, err
		}
		return SessionRunTransition{}, nil
	}
	if err != nil {
		return SessionRunTransition{}, err
	}
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE session_runs SET status = ?, started_at = ?, updated_at = ? WHERE id = ? AND status = ?`, SessionRunStatusRunning, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), run.ID, SessionRunStatusQueued)
	if err != nil {
		return SessionRunTransition{}, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return SessionRunTransition{}, ErrSessionRunTransitionConflict
	}
	run.Status, run.StartedAt, run.UpdatedAt = SessionRunStatusRunning, now, now
	aggregateEvent, err := NewSessionRunTransitionEvent(run)
	if err != nil {
		return SessionRunTransition{}, err
	}
	aggregateEvent, err = appendAggregateEventTx(ctx, tx, aggregateEvent, session.AggregateVersion)
	if err != nil {
		return SessionRunTransition{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET aggregate_version = ? WHERE id = ?`, aggregateEvent.Version, sessionID); err != nil {
		return SessionRunTransition{}, fmt.Errorf("update session run version: %w", err)
	}
	event, err := appendRunEventTx(ctx, tx, run, "session.run.started", nil, now)
	if err != nil {
		return SessionRunTransition{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SessionRunTransition{}, err
	}
	return SessionRunTransition{Run: run, Event: event, Changed: true}, nil
}

func (s *SQLiteStore) SettleSessionRun(ctx context.Context, settlement SessionRunSettlement) (SessionRunTransition, error) {
	if !isSessionRunTerminal(settlement.Status) {
		return SessionRunTransition{}, ErrSessionRunTransitionConflict
	}
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return SessionRunTransition{}, err
	}
	defer tx.Rollback()
	run, err := scanSessionRun(tx.QueryRowContext(ctx, `SELECT `+sessionRunColumns+` FROM session_runs WHERE id = ?`, settlement.ID))
	if errors.Is(err, sql.ErrNoRows) {
		return SessionRunTransition{}, ErrSessionRunNotFound
	}
	if err != nil {
		return SessionRunTransition{}, err
	}
	if isSessionRunTerminal(run.Status) {
		if run.Status == settlement.Status && run.ErrorType == settlement.ErrorType && run.ErrorMessage == settlement.ErrorMessage {
			if err := tx.Commit(ctx); err != nil {
				return SessionRunTransition{}, err
			}
			return SessionRunTransition{Run: run}, nil
		}
		return SessionRunTransition{}, ErrSessionRunTransitionConflict
	}
	if run.Status != settlement.ExpectedStatus || !legalSessionRunSettlement(run.Status, settlement.Status) {
		return SessionRunTransition{}, ErrSessionRunTransitionConflict
	}
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE session_runs SET status = ?, error_type = ?, error_message = ?, completed_at = ?, updated_at = ? WHERE id = ? AND status = ?`, settlement.Status, nullableString(settlement.ErrorType), nullableString(settlement.ErrorMessage), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), run.ID, run.Status)
	if err != nil {
		return SessionRunTransition{}, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return SessionRunTransition{}, ErrSessionRunTransitionConflict
	}
	run.Status, run.ErrorType, run.ErrorMessage, run.CompletedAt, run.UpdatedAt = settlement.Status, settlement.ErrorType, settlement.ErrorMessage, now, now
	session, err := getSessionTx(ctx, tx, run.SessionID)
	if err != nil {
		return SessionRunTransition{}, err
	}
	aggregateEvent, err := NewSessionRunTransitionEvent(run)
	if err != nil {
		return SessionRunTransition{}, err
	}
	aggregateEvent, err = appendAggregateEventTx(ctx, tx, aggregateEvent, session.AggregateVersion)
	if err != nil {
		return SessionRunTransition{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET aggregate_version = ? WHERE id = ?`, aggregateEvent.Version, run.SessionID); err != nil {
		return SessionRunTransition{}, fmt.Errorf("update session run version: %w", err)
	}
	event, err := appendRunEventTx(ctx, tx, run, "session.run."+settlement.Status, settlement.EventData, now)
	if err != nil {
		return SessionRunTransition{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SessionRunTransition{}, err
	}
	return SessionRunTransition{Run: run, Event: event, Changed: true}, nil
}

func (s *SQLiteStore) ListRunningSessionRuns(ctx context.Context) ([]SessionRun, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sessionRunColumns+` FROM session_runs WHERE status = ? ORDER BY session_id, sequence`, SessionRunStatusRunning)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionRun{}
	for rows.Next() {
		run, err := scanSessionRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListQueuedSessionRunSessions(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT session_id FROM session_runs WHERE status = ?`, SessionRunStatusQueued)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) CountQueuedSessionRuns(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM session_runs WHERE status = ?`, SessionRunStatusQueued).Scan(&count)
	return count, err
}

func isSessionRunTerminal(status string) bool {
	return status == SessionRunStatusCompleted || status == SessionRunStatusFailed || status == SessionRunStatusAborted
}
func legalSessionRunSettlement(from, to string) bool {
	return (from == SessionRunStatusRunning && isSessionRunTerminal(to)) || (from == SessionRunStatusQueued && to == SessionRunStatusAborted)
}

func appendRunEventTx(ctx context.Context, tx *immediateTx, run SessionRun, typ string, extra map[string]any, now time.Time) (SessionEvent, error) {
	data := make(map[string]any, len(extra)+6)
	for k, v := range extra {
		data[k] = v
	}
	data["run_id"], data["status"], data["error_type"], data["error_message"] = run.ID, run.Status, run.ErrorType, run.ErrorMessage
	if !run.StartedAt.IsZero() {
		data["started_at"] = run.StartedAt.UTC().Format(time.RFC3339Nano)
	}
	if !run.CompletedAt.IsZero() {
		data["completed_at"] = run.CompletedAt.UTC().Format(time.RFC3339Nano)
	}
	if !run.UpdatedAt.IsZero() {
		data["updated_at"] = run.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return SessionEvent{}, err
	}
	var max sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(seq) FROM session_events WHERE session_id = ?`, run.SessionID).Scan(&max); err != nil {
		return SessionEvent{}, err
	}
	event := SessionEvent{ID: NewID(PrefixEvent), SchemaVersion: 1, Type: typ, Time: now, SessionID: run.SessionID, Seq: max.Int64 + 1, DataJSON: payload, Data: payload}
	if _, err := tx.ExecContext(ctx, `INSERT INTO session_events (id, session_id, seq, schema_version, type, data_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, event.ID, event.SessionID, event.Seq, event.SchemaVersion, event.Type, string(payload), now.Format(time.RFC3339Nano)); err != nil {
		return SessionEvent{}, fmt.Errorf("insert run session event: %w", err)
	}
	return event, nil
}

func nullableJSON(v []byte) any {
	if len(v) == 0 {
		return nil
	}
	return string(v)
}

func scanSessionRun(row rowScanner) (SessionRun, error) {
	var run SessionRun
	var agentJSON string
	var workDir, workspaceID, clientID, schema, errorType, errorMessage, started, completed sql.NullString
	var created, updated string
	if err := row.Scan(&run.ID, &run.SessionID, &run.RequestID, &run.RequestHash, &run.AdmittedVersion, &workDir, &workspaceID, &clientID, &run.Sequence, &run.Status, &run.Message, &agentJSON, &schema, &errorType, &errorMessage, &created, &started, &completed, &updated); err != nil {
		return SessionRun{}, err
	}
	if err := json.Unmarshal([]byte(agentJSON), &run.Agent); err != nil {
		return SessionRun{}, fmt.Errorf("unmarshal run agent: %w", err)
	}
	if schema.Valid {
		run.OutputSchemaJSON = []byte(schema.String)
	}
	run.ErrorType, run.ErrorMessage = errorType.String, errorMessage.String
	run.WorkDir, run.WorkspaceID, run.ClientID = workDir.String, workspaceID.String, clientID.String
	run.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	run.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if started.Valid {
		run.StartedAt, _ = time.Parse(time.RFC3339Nano, started.String)
	}
	if completed.Valid {
		run.CompletedAt, _ = time.Parse(time.RFC3339Nano, completed.String)
	}
	return run, nil
}

// AppendSessionEvent stores one durable session event and assigns the next
// session-scoped sequence number.
func (s *SQLiteStore) AppendSessionEvent(ctx context.Context, event SessionEvent) (SessionEvent, error) {
	if event.ID == "" {
		event.ID = NewID(PrefixEvent)
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	if event.SchemaVersion == 0 {
		event.SchemaVersion = 1
	}
	if len(event.DataJSON) == 0 && len(event.Data) > 0 {
		event.DataJSON = []byte(event.Data)
	}
	if len(event.DataJSON) == 0 {
		event.DataJSON = []byte(`{}`)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SessionEvent{}, err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM sessions WHERE id = ?`, event.SessionID).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return SessionEvent{}, ErrSessionNotFound
		}
		return SessionEvent{}, err
	}

	var seq sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(seq) FROM session_events WHERE session_id = ?`, event.SessionID).Scan(&seq); err != nil {
		return SessionEvent{}, err
	}
	if seq.Valid {
		event.Seq = seq.Int64 + 1
	} else {
		event.Seq = 1
	}
	createdAt := event.Time.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO session_events (id, session_id, seq, schema_version, type, data_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, event.ID, event.SessionID, event.Seq, event.SchemaVersion, event.Type, string(event.DataJSON), createdAt); err != nil {
		return SessionEvent{}, fmt.Errorf("insert session event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return SessionEvent{}, err
	}
	event.Data = json.RawMessage(event.DataJSON)
	return event, nil
}

// ListSessionEvents returns durable session events with Seq > after.
func (s *SQLiteStore) ListSessionEvents(ctx context.Context, sessionID string, after int64, limit int) ([]SessionEvent, error) {
	if err := s.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, seq, schema_version, type, data_json, created_at
		FROM session_events
		WHERE session_id = ? AND seq > ?
		ORDER BY seq ASC
		LIMIT ?
	`, sessionID, after, limit)
	if err != nil {
		return nil, fmt.Errorf("query session events: %w", err)
	}
	defer rows.Close()

	out := []SessionEvent{}
	for rows.Next() {
		var ev SessionEvent
		var dataJSON, createdAt string
		if err := rows.Scan(&ev.ID, &ev.SessionID, &ev.Seq, &ev.SchemaVersion, &ev.Type, &dataJSON, &createdAt); err != nil {
			return nil, err
		}
		ev.DataJSON = []byte(dataJSON)
		ev.Data = json.RawMessage(ev.DataJSON)
		ev.Time, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SessionEventWatermark(ctx context.Context, sessionID string) (int64, error) {
	if err := s.sessionExists(ctx, sessionID); err != nil {
		return 0, err
	}
	var watermark int64
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) FROM session_events WHERE session_id = ?`, sessionID).Scan(&watermark); err != nil {
		return 0, fmt.Errorf("read session event watermark: %w", err)
	}
	return watermark, nil
}

func validatePermissionRequestOwnershipTx(ctx context.Context, tx *immediateTx, request PermissionRequest) error {
	if request.RunID != "" {
		var sessionID string
		if err := tx.QueryRowContext(ctx, `SELECT session_id FROM session_runs WHERE id = ?`, request.RunID).Scan(&sessionID); err != nil || sessionID != request.SessionID {
			return fmt.Errorf("session run %s does not belong to session %s", request.RunID, request.SessionID)
		}
	}
	if request.ToolUseID != "" {
		var sessionID, runID, callID string
		if err := tx.QueryRowContext(ctx, `SELECT session_id, COALESCE(run_id, ''), call_id FROM tool_uses WHERE id = ?`, request.ToolUseID).Scan(&sessionID, &runID, &callID); err != nil || sessionID != request.SessionID {
			return fmt.Errorf("tool use %s does not belong to session %s", request.ToolUseID, request.SessionID)
		}
		if request.RunID != "" && runID != request.RunID {
			return fmt.Errorf("tool use %s does not belong to run %s", request.ToolUseID, request.RunID)
		}
		if request.CallID != "" && callID != "" && callID != request.CallID {
			return fmt.Errorf("tool use %s does not belong to call %s", request.ToolUseID, request.CallID)
		}
	}
	return nil
}

func legalPermissionResolution(status, response string) bool {
	switch response {
	case PermissionResponseOnce, PermissionResponseAlways:
		return status == PermissionRequestStatusApproved
	case PermissionResponseReject:
		return status == PermissionRequestStatusRejected
	case "":
		return status == PermissionRequestStatusTimedOut || status == PermissionRequestStatusInterrupted
	}
	return false
}

func samePermissionResolution(request PermissionRequest, resolution PermissionRequestResolution) bool {
	return request.Status == resolution.Status && request.Response == resolution.Response && request.ErrorType == resolution.ErrorType && request.ErrorMessage == resolution.ErrorMessage
}

func samePendingPermissionRequest(existing, request PermissionRequest) bool {
	return existing.SessionID == request.SessionID && existing.RunID == request.RunID && existing.ToolUseID == request.ToolUseID && existing.CallID == request.CallID && existing.Action == request.Action && existing.Status == PermissionRequestStatusPending && request.Status == PermissionRequestStatusPending && slices.Equal(existing.Resources, request.Resources)
}

func appendPermissionRequestAggregateTx(ctx context.Context, tx *immediateTx, session *Session, request PermissionRequest, resolved bool) (*Session, error) {
	var (
		event AggregateEvent
		err   error
	)
	if resolved {
		event, err = NewSessionPermissionResolutionEvent(request)
	} else {
		event, err = NewSessionPermissionRequestEvent(request)
	}
	if err != nil {
		return nil, err
	}
	event.ClientID = session.ClientID
	event, err = appendAggregateEventTx(ctx, tx, event, session.AggregateVersion)
	if err != nil {
		return nil, err
	}
	return projectSessionEvent(session, event)
}

func appendPermissionGrantAggregateTx(ctx context.Context, tx *immediateTx, session *Session, grant PermissionGrant) (*Session, error) {
	event, err := NewSessionPermissionGrantCreatedEvent(grant)
	if err != nil {
		return nil, err
	}
	event.ClientID = session.ClientID
	event, err = appendAggregateEventTx(ctx, tx, event, session.AggregateVersion)
	if err != nil {
		return nil, err
	}
	return projectSessionEvent(session, event)
}

func appendPermissionEventTx(ctx context.Context, tx *immediateTx, request PermissionRequest, typ string, now time.Time) (SessionEvent, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return SessionEvent{}, fmt.Errorf("marshal permission event: %w", err)
	}
	var max sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(seq) FROM session_events WHERE session_id = ?`, request.SessionID).Scan(&max); err != nil {
		return SessionEvent{}, err
	}
	seq := int64(1)
	if max.Valid {
		seq = max.Int64 + 1
	}
	event := SessionEvent{ID: NewID(PrefixEvent), SchemaVersion: 1, Type: typ, Time: now, SessionID: request.SessionID, Seq: seq, DataJSON: payload, Data: payload}
	if _, err := tx.ExecContext(ctx, `INSERT INTO session_events (id, session_id, seq, schema_version, type, data_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, event.ID, event.SessionID, event.Seq, event.SchemaVersion, event.Type, string(payload), now.UTC().Format(time.RFC3339Nano)); err != nil {
		return SessionEvent{}, fmt.Errorf("insert permission session event: %w", err)
	}
	return event, nil
}

// ---- auth ----------------------------------------------------------------

// GetAuth returns the singleton auth row, or an empty Auth if unset.
func (s *SQLiteStore) GetAuth() (*Auth, error) {
	var auth Auth
	var providersJSON string

	err := s.db.QueryRow(`SELECT providers_json, updated_at FROM auth WHERE id = 1`).
		Scan(&providersJSON, &auth.UpdatedAt)
	if err == sql.ErrNoRows {
		return &Auth{Providers: make(map[string]AuthCredential)}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(providersJSON), &auth.Providers); err != nil {
		return nil, err
	}
	return &auth, nil
}

// SetAuth writes the singleton auth row, upserting on the fixed id=1.
func (s *SQLiteStore) SetAuth(auth *Auth) error {
	auth.UpdatedAt = Now()
	providers, err := json.Marshal(auth.Providers)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO auth (id, providers_json, updated_at) VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET providers_json = ?, updated_at = ?
	`, string(providers), auth.UpdatedAt, string(providers), auth.UpdatedAt)
	return err
}

// ---- helpers -------------------------------------------------------------

const modelCallColumns = `
	id, session_id, COALESCE(run_id, ''), assistant_message_id, step, attempt, status,
	COALESCE(agent_id, ''), COALESCE(model_ref, ''), COALESCE(provider, ''), COALESCE(provider_request_id, ''), COALESCE(api, ''), COALESCE(model_id, ''),
	COALESCE(finish_reason, ''), COALESCE(stop_reason, ''), COALESCE(error_type, ''), COALESCE(error_message, ''),
	input_tokens, output_tokens, reasoning_tokens, cached_input_tokens, cache_write_tokens, total_tokens,
	context_tokens, context_window, COALESCE(context_percent, 0), cost,
	structured_output_json, metadata_json, started_at, completed_at, created_at, updated_at`

const toolUseColumns = `
	id, session_id, COALESCE(run_id, ''), COALESCE(model_call_id, ''), COALESCE(assistant_message_id, ''), COALESCE(part_id, ''),
	step, ordinal, call_id, name, status, input_json, output, structured_json, metadata_json, error_type, error_message,
	proposed_at, authorized_at, started_at, completed_at, created_at, updated_at`

const sessionRunColumns = `
	id, session_id, request_id, request_hash, admitted_version,
	work_dir, workspace_id, client_id, sequence, status, message, agent_json,
	output_schema_json, error_type, error_message, created_at, started_at, completed_at, updated_at`

// SessionRunRequestHash returns the canonical hash for an admission request.
func SessionRunRequestHash(run SessionRun) (string, error) {
	var schema any
	if len(run.OutputSchemaJSON) != 0 {
		if err := json.Unmarshal(run.OutputSchemaJSON, &schema); err != nil {
			return "", fmt.Errorf("decode run output schema: %w", err)
		}
	}
	agent := run.Agent
	agent.CreatedAt, agent.UpdatedAt = "", ""
	payload, err := json.Marshal(struct {
		Message      string `json:"message"`
		Agent        Agent  `json:"agent"`
		OutputSchema any    `json:"output_schema"`
		ClientID     string `json:"client_id"`
		WorkDir      string `json:"work_dir"`
		WorkspaceID  string `json:"workspace_id"`
	}{run.Message, agent, schema, run.ClientID, run.WorkDir, run.WorkspaceID})
	if err != nil {
		return "", fmt.Errorf("marshal run request: %w", err)
	}
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("%x", sum), nil
}

func (s *SQLiteStore) sessionExists(ctx context.Context, sessionID string) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM sessions WHERE id = ?`, sessionID).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return ErrSessionNotFound
		}
		return err
	}
	return nil
}

// rowScanner is the common subset of *sql.Row and *sql.Rows used by
// scanAgent. Lets us reuse one scan path for QueryRow and rows.Next.
type rowScanner interface {
	Scan(dest ...any) error
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func scanPermissionRequest(r rowScanner) (PermissionRequest, error) {
	var request PermissionRequest
	var resources, resolvedAt sql.NullString
	var createdAt, updatedAt string
	if err := r.Scan(&request.ID, &request.SessionID, &request.RunID, &request.ToolUseID, &request.CallID, &request.Action, &resources, &request.Status, &request.Response, &request.ErrorType, &request.ErrorMessage, &createdAt, &resolvedAt, &updatedAt); err != nil {
		return PermissionRequest{}, err
	}
	if err := json.Unmarshal([]byte(resources.String), &request.Resources); err != nil {
		return PermissionRequest{}, fmt.Errorf("unmarshal permission resources: %w", err)
	}
	request.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	request.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if resolvedAt.Valid {
		request.ResolvedAt, _ = time.Parse(time.RFC3339Nano, resolvedAt.String)
	}
	return request, nil
}

func scanModelCall(r rowScanner) (ModelCall, error) {
	var call ModelCall
	var assistantMessageID, completedAt, structuredOutputJSON, metadataJSON sql.NullString
	var startedAt, createdAt, updatedAt string
	if err := r.Scan(
		&call.ID, &call.SessionID, &call.RunID, &assistantMessageID, &call.Step, &call.Attempt, &call.Status,
		&call.AgentID, &call.ModelRef, &call.Provider, &call.ProviderRequestID, &call.API, &call.ModelID,
		&call.FinishReason, &call.StopReason, &call.ErrorType, &call.ErrorMessage,
		&call.InputTokens, &call.OutputTokens, &call.ReasoningTokens, &call.CachedInputTokens, &call.CacheWriteTokens, &call.TotalTokens,
		&call.ContextTokens, &call.ContextWindow, &call.ContextPercent, &call.Cost,
		&structuredOutputJSON, &metadataJSON, &startedAt, &completedAt, &createdAt, &updatedAt,
	); err != nil {
		return ModelCall{}, err
	}
	if assistantMessageID.Valid {
		call.AssistantMessageID = assistantMessageID.String
	}
	if structuredOutputJSON.Valid {
		call.StructuredOutputJSON = []byte(structuredOutputJSON.String)
	}
	if metadataJSON.Valid {
		call.MetadataJSON = []byte(metadataJSON.String)
		call.Trace = json.RawMessage(call.MetadataJSON)
	}
	call.StartedAt, _ = time.Parse(time.RFC3339Nano, startedAt)
	if completedAt.Valid {
		call.CompletedAt, _ = time.Parse(time.RFC3339Nano, completedAt.String)
	}
	call.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	call.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return call, nil
}

func toolUseSessionExists(ctx context.Context, tx *immediateTx, sessionID string) error {
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM sessions WHERE id = ?`, sessionID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSessionNotFound
		}
		return err
	}
	return nil
}

func scanToolUse(r rowScanner) (ToolUse, error) {
	var use ToolUse
	var input, output, structured, metadata, errorType, errorMessage, authorizedAt, startedAt, completedAt sql.NullString
	var proposedAt, createdAt, updatedAt string
	err := r.Scan(&use.ID, &use.SessionID, &use.RunID, &use.ModelCallID, &use.AssistantMessageID, &use.PartID,
		&use.Step, &use.Ordinal, &use.CallID, &use.Name, &use.Status, &input, &output, &structured, &metadata, &errorType, &errorMessage,
		&proposedAt, &authorizedAt, &startedAt, &completedAt, &createdAt, &updatedAt)
	if err != nil {
		return ToolUse{}, err
	}
	if input.Valid {
		use.InputJSON = []byte(input.String)
	}
	if output.Valid {
		use.Output = output.String
	}
	if structured.Valid {
		use.StructuredJSON = []byte(structured.String)
	}
	if metadata.Valid {
		use.MetadataJSON = []byte(metadata.String)
	}
	if errorType.Valid {
		use.ErrorType = errorType.String
	}
	if errorMessage.Valid {
		use.ErrorMessage = errorMessage.String
	}
	use.ProposedAt, _ = time.Parse(time.RFC3339Nano, proposedAt)
	if authorizedAt.Valid {
		use.AuthorizedAt, _ = time.Parse(time.RFC3339Nano, authorizedAt.String)
	}
	if startedAt.Valid {
		use.StartedAt, _ = time.Parse(time.RFC3339Nano, startedAt.String)
	}
	if completedAt.Valid {
		use.CompletedAt, _ = time.Parse(time.RFC3339Nano, completedAt.String)
	}
	use.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	use.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return use, nil
}

func insertToolUse(ctx context.Context, tx *immediateTx, use ToolUse) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO tool_uses (`+toolUseColumnsInsert+`) VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, toolUseArgs(use)...)
	return err
}

func updateToolUse(ctx context.Context, tx *immediateTx, use ToolUse) error {
	_, err := tx.ExecContext(ctx, `UPDATE tool_uses SET status = ?, input_json = ?, output = ?, structured_json = ?, metadata_json = ?, error_type = ?, error_message = ?, authorized_at = ?, started_at = ?, completed_at = ?, updated_at = ? WHERE id = ?`,
		use.Status, nullableBytes(use.InputJSON), nullableString(use.Output), nullableBytes(use.StructuredJSON), nullableBytes(use.MetadataJSON), nullableString(use.ErrorType), nullableString(use.ErrorMessage), nullableTime(use.AuthorizedAt), nullableTime(use.StartedAt), nullableTime(use.CompletedAt), nullableTime(use.UpdatedAt), use.ID)
	return err
}

const toolUseColumnsInsert = `id, session_id, run_id, model_call_id, assistant_message_id, part_id, step, ordinal, call_id, name, status, input_json, output, structured_json, metadata_json, error_type, error_message, proposed_at, authorized_at, started_at, completed_at, created_at, updated_at`

func toolUseArgs(use ToolUse) []any {
	return []any{use.ID, use.SessionID, use.RunID, use.ModelCallID, use.AssistantMessageID, use.PartID, use.Step, use.Ordinal, use.CallID, use.Name, use.Status, nullableBytes(use.InputJSON), nullableString(use.Output), nullableBytes(use.StructuredJSON), nullableBytes(use.MetadataJSON), nullableString(use.ErrorType), nullableString(use.ErrorMessage), nullableTime(use.ProposedAt), nullableTime(use.AuthorizedAt), nullableTime(use.StartedAt), nullableTime(use.CompletedAt), nullableTime(use.CreatedAt), nullableTime(use.UpdatedAt)}
}

func nullableString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func nullableTime(v time.Time) *string {
	if v.IsZero() {
		return nil
	}
	s := v.UTC().Format(time.RFC3339Nano)
	return &s
}

func sameToolUseIdentity(a, b ToolUse) bool {
	return a.SessionID == b.SessionID && a.RunID == b.RunID && a.ModelCallID == b.ModelCallID && a.AssistantMessageID == b.AssistantMessageID && a.PartID == b.PartID && a.Step == b.Step && a.Ordinal == b.Ordinal && a.CallID == b.CallID && a.Name == b.Name
}

func sameToolUse(a, b ToolUse) bool {
	return sameToolUseIdentity(a, b) && a.Status == b.Status && bytes.Equal(a.InputJSON, b.InputJSON) && a.Output == b.Output && bytes.Equal(a.StructuredJSON, b.StructuredJSON) && bytes.Equal(a.MetadataJSON, b.MetadataJSON) && a.ErrorType == b.ErrorType && a.ErrorMessage == b.ErrorMessage && a.ProposedAt.Equal(b.ProposedAt) && a.AuthorizedAt.Equal(b.AuthorizedAt) && a.StartedAt.Equal(b.StartedAt) && a.CompletedAt.Equal(b.CompletedAt) && a.CreatedAt.Equal(b.CreatedAt) && a.UpdatedAt.Equal(b.UpdatedAt)
}

func legalToolUseTransition(from, to string) bool {
	switch from {
	case ToolUseStatusProposed:
		return to == ToolUseStatusAuthorized || to == ToolUseStatusDeclined || to == ToolUseStatusFailed || to == ToolUseStatusInterrupted
	case ToolUseStatusAuthorized:
		return to == ToolUseStatusStarted || to == ToolUseStatusFailed || to == ToolUseStatusInterrupted
	case ToolUseStatusStarted:
		return to == ToolUseStatusCompleted || to == ToolUseStatusFailed || to == ToolUseStatusInterrupted
	}
	return false
}

func isToolUseTerminal(status string) bool {
	return status == ToolUseStatusCompleted || status == ToolUseStatusFailed || status == ToolUseStatusInterrupted || status == ToolUseStatusDeclined
}

// scanAgent reads one agent row from any rowScanner.
func scanAgent(r rowScanner) (*Agent, error) {
	var a Agent
	var toolsJSON string
	var permissionsJSON sql.NullString
	var optionsJSON sql.NullString
	var outputSchemaJSON sql.NullString

	if err := r.Scan(
		&a.ID, &a.Name, &a.Instructions, &toolsJSON, &permissionsJSON,
		&a.ModelRef, &optionsJSON, &outputSchemaJSON,
		&a.CreatedAt, &a.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if toolsJSON != "" {
		if err := json.Unmarshal([]byte(toolsJSON), &a.Tools); err != nil {
			return nil, err
		}
	}
	if permissionsJSON.Valid && permissionsJSON.String != "" {
		if err := json.Unmarshal([]byte(permissionsJSON.String), &a.Permissions); err != nil {
			return nil, err
		}
	}
	if optionsJSON.Valid && optionsJSON.String != "" {
		if err := json.Unmarshal([]byte(optionsJSON.String), &a.Options); err != nil {
			return nil, err
		}
	}
	if outputSchemaJSON.Valid && outputSchemaJSON.String != "" {
		if err := json.Unmarshal([]byte(outputSchemaJSON.String), &a.OutputSchema); err != nil {
			return nil, err
		}
	}
	return &a, nil
}

// marshalNullable returns a *string for use as a nullable SQL column:
// nil if v is nil/empty, else a pointer to the JSON encoding.
func marshalNullable(v any) (*string, error) {
	if v == nil {
		return nil, nil
	}
	// Treat empty maps as null too, to keep the DB tidy.
	if m, ok := v.(map[string]any); ok && len(m) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	str := string(b)
	return &str, nil
}

func nullableBytes(b []byte) *string {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	return &s
}
