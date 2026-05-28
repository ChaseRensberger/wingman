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

// DefaultDBPath returns the platform-appropriate default DB location.
func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "wingman", "wingman.db"), nil
}

// Now returns the current UTC timestamp formatted as RFC3339. Centralized
// so tests can swap it out later if needed.
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

	optionsJSON, err := marshalNullable(agent.Options)
	if err != nil {
		return err
	}
	outputSchemaJSON, err := marshalNullable(agent.OutputSchema)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
		INSERT INTO agents (id, name, instructions, tools_json, model_ref, options_json, output_schema_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, agent.ID, agent.Name, agent.Instructions, string(tools), agent.ModelRef, optionsJSON, outputSchemaJSON, agent.CreatedAt, agent.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert agent: %w", err)
	}
	return nil
}

// GetAgent returns the agent with the given ID, or an error if not found.
func (s *SQLiteStore) GetAgent(id string) (*Agent, error) {
	row := s.db.QueryRow(`
		SELECT id, name, instructions, tools_json, model_ref, options_json, output_schema_json, created_at, updated_at
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
		SELECT id, name, instructions, tools_json, model_ref, options_json, output_schema_json, created_at, updated_at
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
	optionsJSON, err := marshalNullable(agent.Options)
	if err != nil {
		return err
	}
	outputSchemaJSON, err := marshalNullable(agent.OutputSchema)
	if err != nil {
		return err
	}

	res, err := s.db.Exec(`
		UPDATE agents SET name = ?, instructions = ?, tools_json = ?, model_ref = ?, options_json = ?, output_schema_json = ?, updated_at = ?
		WHERE id = ?
	`, agent.Name, agent.Instructions, string(tools), agent.ModelRef, optionsJSON, outputSchemaJSON, agent.UpdatedAt, agent.ID)
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

// CreateSession inserts a session row. Runs in a single transaction so
// partial creation never happens.
func (s *SQLiteStore) CreateSession(session *Session) error {
	if session.ID == "" {
		session.ID = NewID(PrefixSession)
	}
	now := Now()
	session.CreatedAt = now
	session.UpdatedAt = now

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if session.ClientID != "" {
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM clients WHERE id = ?`, session.ClientID).Scan(&exists); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("client not found: %s", session.ClientID)
			}
			return fmt.Errorf("verify client: %w", err)
		}
	}
	if session.WorkspaceID != "" {
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM workspaces WHERE id = ?`, session.WorkspaceID).Scan(&exists); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("workspace not found: %s", session.WorkspaceID)
			}
			return fmt.Errorf("verify workspace: %w", err)
		}
	}

	var workDirPtr *string
	if session.WorkDir != "" {
		workDirPtr = &session.WorkDir
	}
	var workspaceIDPtr *string
	if session.WorkspaceID != "" {
		workspaceIDPtr = &session.WorkspaceID
	}
	var clientIDPtr *string
	if session.ClientID != "" {
		clientIDPtr = &session.ClientID
	}
	if _, err := tx.Exec(`
		INSERT INTO sessions (id, title, work_dir, workspace_id, client_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, session.ID, session.Title, workDirPtr, workspaceIDPtr, clientIDPtr, session.CreatedAt, session.UpdatedAt); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}

	return tx.Commit()
}

// GetSession returns the session metadata.
func (s *SQLiteStore) GetSession(id string) (*Session, error) {
	var session Session
	var workDir sql.NullString
	var workspaceID sql.NullString
	var clientID sql.NullString
	err := s.db.QueryRow(`
		SELECT id, title, work_dir, workspace_id, client_id, created_at, updated_at FROM sessions WHERE id = ?
	`, id).Scan(&session.ID, &session.Title, &workDir, &workspaceID, &clientID, &session.CreatedAt, &session.UpdatedAt)
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
		SELECT id, title, work_dir, workspace_id, client_id, created_at, updated_at FROM sessions ORDER BY created_at DESC
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
		if err := rows.Scan(&sess.ID, &sess.Title, &workDir, &workspaceID, &clientID, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
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
		SELECT id, title, work_dir, workspace_id, client_id, created_at, updated_at FROM sessions WHERE client_id = ? ORDER BY created_at DESC
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
		if err := rows.Scan(&sess.ID, &sess.Title, &workDir, &workspaceID, &cid, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
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
		SELECT id, title, work_dir, workspace_id, client_id, created_at, updated_at FROM sessions WHERE workspace_id = ? ORDER BY created_at DESC
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
		if err := rows.Scan(&sess.ID, &sess.Title, &workDir, &sid, &cid, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
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

// UpdateSession overwrites the session's mutable metadata.
func (s *SQLiteStore) UpdateSession(session *Session) error {
	session.UpdatedAt = Now()
	if session.WorkspaceID != "" {
		var exists int
		if err := s.db.QueryRow(`SELECT 1 FROM workspaces WHERE id = ?`, session.WorkspaceID).Scan(&exists); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("workspace not found: %s", session.WorkspaceID)
			}
			return fmt.Errorf("verify workspace: %w", err)
		}
	}
	var workDirPtr *string
	if session.WorkDir != "" {
		workDirPtr = &session.WorkDir
	}
	var workspaceIDPtr *string
	if session.WorkspaceID != "" {
		workspaceIDPtr = &session.WorkspaceID
	}

	res, err := s.db.Exec(`
		UPDATE sessions SET title = ?, work_dir = ?, workspace_id = ?, updated_at = ? WHERE id = ?
	`, session.Title, workDirPtr, workspaceIDPtr, session.UpdatedAt, session.ID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("session not found: %s", session.ID)
	}
	return nil
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
	startedAt := call.StartedAt.UTC().Format(time.RFC3339)
	var completedAt *string
	if !call.CompletedAt.IsZero() {
		v := call.CompletedAt.UTC().Format(time.RFC3339)
		completedAt = &v
	}
	createdAt := call.CreatedAt.UTC().Format(time.RFC3339)
	updatedAt := call.UpdatedAt.UTC().Format(time.RFC3339)

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
	}
	call.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	if completedAt.Valid {
		call.CompletedAt, _ = time.Parse(time.RFC3339, completedAt.String)
	}
	call.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	call.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return call, nil
}

// scanAgent reads one agent row from any rowScanner.
func scanAgent(r rowScanner) (*Agent, error) {
	var a Agent
	var toolsJSON string
	var optionsJSON sql.NullString
	var outputSchemaJSON sql.NullString

	if err := r.Scan(
		&a.ID, &a.Name, &a.Instructions, &toolsJSON,
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
