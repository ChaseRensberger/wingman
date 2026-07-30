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
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	if !dbExists {
		if err := store.seedDefaultAgents(); err != nil {
			db.Close()
			return nil, fmt.Errorf("seed default agents: %w", err)
		}
	}

	return store, nil
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
		ID:        NewID(PrefixClient),
		Name:      name,
		CreatedAt: Now(),
	}
	_, err = s.db.Exec(`
		INSERT INTO clients (id, name, created_at) VALUES (?, ?, ?)
	`, client.ID, client.Name, client.CreatedAt)
	if err != nil {
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

// DeleteSession removes the session and (via ON DELETE CASCADE) all of
// its messages and parts.
func (s *SQLiteStore) DeleteSession(id string) error {
	res, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("session not found: %s", id)
	}
	return nil
}

// UpsertMessage inserts or updates a message row keyed by ID.
// Does not touch parts. Idx and created_at are preserved on update.
func (s *SQLiteStore) UpsertMessage(ctx context.Context, msg StoredMessage) error {
	createdAt := msg.CreatedAt.UTC().Format(time.RFC3339)
	updatedAt := msg.UpdatedAt.UTC().Format(time.RFC3339)

	var metadataJSON *string
	if msg.MetadataJSON != nil {
		s := string(msg.MetadataJSON)
		metadataJSON = &s
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO messages (id, session_id, idx, role, metadata_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			role = excluded.role,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at
	`, msg.ID, msg.SessionID, msg.Idx, msg.Role, metadataJSON, createdAt, updatedAt)
	if err != nil {
		return fmt.Errorf("upsert message: %w", err)
	}
	return nil
}

// UpsertPart inserts or updates a part row keyed by ID.
// Sequence (mapped to idx) and created_at are preserved on update.
func (s *SQLiteStore) UpsertPart(ctx context.Context, part StoredPart) error {
	createdAt := part.CreatedAt.UTC().Format(time.RFC3339)
	updatedAt := part.UpdatedAt.UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO parts (id, message_id, idx, kind, payload_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			kind = excluded.kind,
			payload_json = excluded.payload_json,
			updated_at = excluded.updated_at
	`, part.ID, part.MessageID, part.Sequence, part.Kind, string(part.PayloadJSON), createdAt, updatedAt)
	if err != nil {
		return fmt.Errorf("upsert part: %w", err)
	}
	return nil
}

// ListMessages returns all messages for the session ordered by Idx ASC,
// with each message's Parts populated and ordered by Sequence (idx) ASC.
// Returns ErrSessionNotFound if the session does not exist.
// Returns an empty slice (not nil) when the session has no messages.
func (s *SQLiteStore) ListMessages(ctx context.Context, sessionID string) ([]StoredMessage, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM sessions WHERE id = ?`, sessionID).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, idx, role, metadata_json, created_at, updated_at
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
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Idx, &m.Role, &metadataJSON, &createdAt, &updatedAt); err != nil {
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

	partRows, err := s.db.QueryContext(ctx, `
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

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO model_calls (
			id, session_id, assistant_message_id, step, attempt, status,
			agent_id, model_ref, provider, api, model_id,
			finish_reason, stop_reason, error_type, error_message,
			input_tokens, output_tokens, reasoning_tokens, cached_input_tokens, cache_write_tokens, total_tokens,
			context_tokens, context_window, context_percent, cost,
			structured_output_json, metadata_json, started_at, completed_at, created_at, updated_at
		)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, step, attempt) DO UPDATE SET
			assistant_message_id = excluded.assistant_message_id,
			status = excluded.status,
			agent_id = excluded.agent_id,
			model_ref = excluded.model_ref,
			provider = excluded.provider,
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
	`, call.ID, call.SessionID, call.AssistantMessageID, call.Step, call.Attempt, call.Status,
		call.AgentID, call.ModelRef, call.Provider, call.API, call.ModelID,
		call.FinishReason, call.StopReason, call.ErrorType, call.ErrorMessage,
		call.InputTokens, call.OutputTokens, call.ReasoningTokens, call.CachedInputTokens, call.CacheWriteTokens, call.TotalTokens,
		call.ContextTokens, call.ContextWindow, call.ContextPercent, call.Cost,
		nullableBytes(call.StructuredOutputJSON), nullableBytes(call.MetadataJSON), startedAt, completedAt, createdAt, updatedAt)
	if err != nil {
		return fmt.Errorf("upsert model call: %w", err)
	}
	return nil
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
		ORDER BY step DESC, attempt DESC
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

// ListModelCalls returns all model calls for the session ordered by step.
func (s *SQLiteStore) ListModelCalls(ctx context.Context, sessionID string) ([]ModelCall, error) {
	if err := s.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+modelCallColumns+`
		FROM model_calls
		WHERE session_id = ?
		ORDER BY step ASC, attempt ASC
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

func (s *SQLiteStore) CreateSessionRun(ctx context.Context, run SessionRun) (SessionRun, error) {
	if run.ID == "" {
		run.ID = NewID(PrefixRun)
	}
	agentJSON, err := json.Marshal(run.Agent)
	if err != nil {
		return SessionRun{}, fmt.Errorf("marshal run agent: %w", err)
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SessionRun{}, err
	}
	defer tx.Rollback()
	var next int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence), 0) + 1 FROM session_runs WHERE session_id = ?`, run.SessionID).Scan(&next); err != nil {
		return SessionRun{}, err
	}
	run.Sequence = next
	run.Status = SessionRunStatusQueued
	run.CreatedAt = now
	run.UpdatedAt = now
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO session_runs (id, session_id, sequence, status, message, agent_json, output_schema_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, run.ID, run.SessionID, run.Sequence, run.Status, run.Message, string(agentJSON), nullableJSON(run.OutputSchemaJSON), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		return SessionRun{}, fmt.Errorf("insert session run: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return SessionRun{}, err
	}
	return run, nil
}

func (s *SQLiteStore) ClaimNextSessionRun(ctx context.Context, sessionID string) (*SessionRun, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	run, err := scanSessionRun(tx.QueryRowContext(ctx, `
		SELECT id, session_id, sequence, status, message, agent_json, output_schema_json, error_message, created_at, started_at, completed_at, updated_at
		FROM session_runs WHERE session_id = ? AND status = ? ORDER BY sequence LIMIT 1
	`, sessionID, SessionRunStatusQueued))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE session_runs SET status = ?, started_at = ?, updated_at = ? WHERE id = ?`, SessionRunStatusRunning, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), run.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	run.Status, run.StartedAt, run.UpdatedAt = SessionRunStatusRunning, now, now
	return &run, nil
}

func (s *SQLiteStore) CompleteSessionRun(ctx context.Context, id, status, errorMessage string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `UPDATE session_runs SET status = ?, error_message = ?, completed_at = ?, updated_at = ? WHERE id = ?`, status, errorMessage, now, now, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrSessionNotFound
	}
	return nil
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

func (s *SQLiteStore) AbortRunningSessionRuns(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `UPDATE session_runs SET status = ?, error_message = ?, completed_at = ?, updated_at = ? WHERE status = ?`, SessionRunStatusAborted, "server shutdown", now, now, SessionRunStatusRunning)
	return err
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
	var schema, errorMessage, started, completed sql.NullString
	var created, updated string
	if err := row.Scan(&run.ID, &run.SessionID, &run.Sequence, &run.Status, &run.Message, &agentJSON, &schema, &errorMessage, &created, &started, &completed, &updated); err != nil {
		return SessionRun{}, err
	}
	if err := json.Unmarshal([]byte(agentJSON), &run.Agent); err != nil {
		return SessionRun{}, fmt.Errorf("unmarshal run agent: %w", err)
	}
	if schema.Valid {
		run.OutputSchemaJSON = []byte(schema.String)
	}
	run.ErrorMessage = errorMessage.String
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
		INSERT INTO session_events (id, session_id, seq, type, data_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, event.ID, event.SessionID, event.Seq, event.Type, string(event.DataJSON), createdAt); err != nil {
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
		SELECT id, session_id, seq, type, data_json, created_at
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
		if err := rows.Scan(&ev.ID, &ev.SessionID, &ev.Seq, &ev.Type, &dataJSON, &createdAt); err != nil {
			return nil, err
		}
		ev.DataJSON = []byte(dataJSON)
		ev.Data = json.RawMessage(ev.DataJSON)
		ev.Time, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, ev)
	}
	return out, rows.Err()
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
	id, session_id, assistant_message_id, step, attempt, status,
	COALESCE(agent_id, ''), COALESCE(model_ref, ''), COALESCE(provider, ''), COALESCE(api, ''), COALESCE(model_id, ''),
	COALESCE(finish_reason, ''), COALESCE(stop_reason, ''), COALESCE(error_type, ''), COALESCE(error_message, ''),
	input_tokens, output_tokens, reasoning_tokens, cached_input_tokens, cache_write_tokens, total_tokens,
	context_tokens, context_window, COALESCE(context_percent, 0), cost,
	structured_output_json, metadata_json, started_at, completed_at, created_at, updated_at`

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

func scanModelCall(r rowScanner) (ModelCall, error) {
	var call ModelCall
	var assistantMessageID, completedAt, structuredOutputJSON, metadataJSON sql.NullString
	var startedAt, createdAt, updatedAt string
	if err := r.Scan(
		&call.ID, &call.SessionID, &assistantMessageID, &call.Step, &call.Attempt, &call.Status,
		&call.AgentID, &call.ModelRef, &call.Provider, &call.API, &call.ModelID,
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
