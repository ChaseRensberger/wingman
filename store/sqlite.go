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
	"fmt"
	"os"
	"path/filepath"
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

	return &SQLiteStore{db: db}, nil
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
		INSERT INTO agents (id, name, instructions, tools_json, provider, model, options_json, output_schema_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, agent.ID, agent.Name, agent.Instructions, string(tools), agent.Provider, agent.Model, optionsJSON, outputSchemaJSON, agent.CreatedAt, agent.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert agent: %w", err)
	}
	return nil
}

// GetAgent returns the agent with the given ID, or an error if not found.
func (s *SQLiteStore) GetAgent(id string) (*Agent, error) {
	row := s.db.QueryRow(`
		SELECT id, name, instructions, tools_json, provider, model, options_json, output_schema_json, created_at, updated_at
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
		SELECT id, name, instructions, tools_json, provider, model, options_json, output_schema_json, created_at, updated_at
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
		UPDATE agents SET name = ?, instructions = ?, tools_json = ?, provider = ?, model = ?, options_json = ?, output_schema_json = ?, updated_at = ?
		WHERE id = ?
	`, agent.Name, agent.Instructions, string(tools), agent.Provider, agent.Model, optionsJSON, outputSchemaJSON, agent.UpdatedAt, agent.ID)
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

// CreateClient inserts a new client row with a fresh KSUID and the
// current RFC3339 timestamp.
func (s *SQLiteStore) CreateClient(name string) (*Client, error) {
	client := &Client{
		ID:        NewID(PrefixClient),
		Name:      name,
		CreatedAt: Now(),
	}
	_, err := s.db.Exec(`
		INSERT INTO clients (id, name, created_at) VALUES (?, ?, ?)
	`, client.ID, client.Name, client.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert client: %w", err)
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

	var workDirPtr *string
	if session.WorkDir != "" {
		workDirPtr = &session.WorkDir
	}
	var clientIDPtr *string
	if session.ClientID != "" {
		clientIDPtr = &session.ClientID
	}
	if _, err := tx.Exec(`
		INSERT INTO sessions (id, title, work_dir, client_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, session.ID, session.Title, workDirPtr, clientIDPtr, session.CreatedAt, session.UpdatedAt); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}

	return tx.Commit()
}

// GetSession returns the session metadata.
func (s *SQLiteStore) GetSession(id string) (*Session, error) {
	var session Session
	var workDir sql.NullString
	var clientID sql.NullString
	err := s.db.QueryRow(`
		SELECT id, title, work_dir, client_id, created_at, updated_at FROM sessions WHERE id = ?
	`, id).Scan(&session.ID, &session.Title, &workDir, &clientID, &session.CreatedAt, &session.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	session.WorkDir = workDir.String
	session.ClientID = clientID.String
	return &session, nil
}

// ListSessions returns every session, newest first. History is no longer
// loaded automatically; use ListMessages for message retrieval.
func (s *SQLiteStore) ListSessions() ([]*Session, error) {
	rows, err := s.db.Query(`
		SELECT id, title, work_dir, client_id, created_at, updated_at FROM sessions ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Session
	for rows.Next() {
		var sess Session
		var workDir sql.NullString
		var clientID sql.NullString
		if err := rows.Scan(&sess.ID, &sess.Title, &workDir, &clientID, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
			return nil, err
		}
		sess.WorkDir = workDir.String
		sess.ClientID = clientID.String
		out = append(out, &sess)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListSessionsByClient returns every session belonging to a specific
// client, newest first. Sessions with no client are excluded.
func (s *SQLiteStore) ListSessionsByClient(clientID string) ([]*Session, error) {
	rows, err := s.db.Query(`
		SELECT id, title, work_dir, client_id, created_at, updated_at FROM sessions WHERE client_id = ? ORDER BY created_at DESC
	`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Session
	for rows.Next() {
		var sess Session
		var workDir sql.NullString
		var cid sql.NullString
		if err := rows.Scan(&sess.ID, &sess.Title, &workDir, &cid, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
			return nil, err
		}
		sess.WorkDir = workDir.String
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
	var workDirPtr *string
	if session.WorkDir != "" {
		workDirPtr = &session.WorkDir
	}

	res, err := s.db.Exec(`
		UPDATE sessions SET title = ?, work_dir = ?, updated_at = ? WHERE id = ?
	`, session.Title, workDirPtr, session.UpdatedAt, session.ID)
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

// rowScanner is the common subset of *sql.Row and *sql.Rows used by
// scanAgent. Lets us reuse one scan path for QueryRow and rows.Next.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanAgent reads one agent row from any rowScanner.
func scanAgent(r rowScanner) (*Agent, error) {
	var a Agent
	var toolsJSON string
	var optionsJSON sql.NullString
	var outputSchemaJSON sql.NullString

	if err := r.Scan(
		&a.ID, &a.Name, &a.Instructions, &toolsJSON,
		&a.Provider, &a.Model, &optionsJSON, &outputSchemaJSON,
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
