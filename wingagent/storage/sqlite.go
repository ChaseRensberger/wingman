// Package storage owns wingagent's SQLite-backed persistence: agents,
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
package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/chaserensberger/wingman/wingmodels"
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

// ---- sessions ------------------------------------------------------------

// CreateSession inserts a session row and (if non-empty) the initial
// history. Runs in a single transaction so partial creation never
// happens.
func (s *SQLiteStore) CreateSession(session *Session) error {
	if session.ID == "" {
		session.ID = NewID(PrefixSession)
	}
	now := Now()
	session.CreatedAt = now
	session.UpdatedAt = now
	if session.History == nil {
		session.History = []wingmodels.Message{}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		INSERT INTO sessions (id, title, work_dir, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, session.ID, session.Title, session.WorkDir, session.CreatedAt, session.UpdatedAt); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}

	if err := writeMessages(tx, session.ID, session.History); err != nil {
		return err
	}
	return tx.Commit()
}

// GetSession returns the session with all of its messages and parts.
func (s *SQLiteStore) GetSession(id string) (*Session, error) {
	var session Session
	err := s.db.QueryRow(`
		SELECT id, title, work_dir, created_at, updated_at FROM sessions WHERE id = ?
	`, id).Scan(&session.ID, &session.Title, &session.WorkDir, &session.CreatedAt, &session.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	msgs, err := readMessages(s.db, id)
	if err != nil {
		return nil, err
	}
	session.History = msgs
	return &session, nil
}

// ListSessions returns every session, newest first. History is loaded
// for each; if you have many sessions with deep histories, consider
// adding a ListSessionsMetadata() that omits History (out of scope for
// v0.1).
func (s *SQLiteStore) ListSessions() ([]*Session, error) {
	rows, err := s.db.Query(`
		SELECT id, title, work_dir, created_at, updated_at FROM sessions ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(&sess.ID, &sess.Title, &sess.WorkDir, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &sess)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Hydrate histories. Done outside the rows loop so the read query is
	// closed before the per-session reads run (avoids reentrant queries
	// on the single connection).
	for _, sess := range out {
		msgs, err := readMessages(s.db, sess.ID)
		if err != nil {
			return nil, err
		}
		sess.History = msgs
	}
	return out, nil
}

// UpdateSession overwrites the session's mutable metadata (title,
// work_dir, and updated_at). It does NOT touch the message history —
// use AppendMessage for incremental appends or ReplaceMessages for
// full rewrites. This split prevents the wasteful "delete+rewrite the
// whole transcript on every turn" pattern the original implementation
// forced.
func (s *SQLiteStore) UpdateSession(session *Session) error {
	session.UpdatedAt = Now()

	res, err := s.db.Exec(`
		UPDATE sessions SET title = ?, work_dir = ?, updated_at = ? WHERE id = ?
	`, session.Title, session.WorkDir, session.UpdatedAt, session.ID)
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

// AppendMessage appends a single message (and its parts) to the
// session's history at the next idx. Wrapped in a transaction so
// either every part lands or none do; idx is computed by selecting
// MAX(idx)+1 inside the transaction to keep ordering stable under
// concurrent writers (SQLite WAL serializes via the single conn cap,
// but the SELECT-then-INSERT pattern still requires the txn).
//
// Also bumps the parent session's updated_at so listing/sorting
// reflects activity without the caller having to issue a separate
// UpdateSession.
func (s *SQLiteStore) AppendMessage(sessionID string, msg wingmodels.Message) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Verify the session exists. Without this check a stray
	// AppendMessage call on a deleted session would silently insert
	// orphan rows (foreign-key cascades wouldn't fire because the
	// parent is already gone — actually sqlite would reject the FK,
	// but we want a clearer error).
	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM sessions WHERE id = ?`, sessionID).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("session not found: %s", sessionID)
		}
		return err
	}

	var nextIdx int
	if err := tx.QueryRow(
		`SELECT COALESCE(MAX(idx), -1) + 1 FROM messages WHERE session_id = ?`,
		sessionID,
	).Scan(&nextIdx); err != nil {
		return fmt.Errorf("compute next idx: %w", err)
	}

	now := Now()
	msgID := NewID(PrefixMessage)
	if _, err := tx.Exec(`
		INSERT INTO messages (id, session_id, idx, role, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, msgID, sessionID, nextIdx, string(msg.Role), now); err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	for j, part := range msg.Content {
		payload, err := wingmodels.MarshalPart(part)
		if err != nil {
			return fmt.Errorf("marshal part %d: %w", j, err)
		}
		if _, err := tx.Exec(`
			INSERT INTO parts (id, message_id, idx, kind, payload_json)
			VALUES (?, ?, ?, ?, ?)
		`, NewID(PrefixPart), msgID, j, part.Type(), string(payload)); err != nil {
			return fmt.Errorf("insert part %d: %w", j, err)
		}
	}

	if _, err := tx.Exec(
		`UPDATE sessions SET updated_at = ? WHERE id = ?`,
		now, sessionID,
	); err != nil {
		return fmt.Errorf("bump session updated_at: %w", err)
	}
	return tx.Commit()
}

// ReplaceMessages atomically clears the session's history and writes
// msgs in order. Reserved for rehydration / history-edit tools; do
// not call this in routine message paths (use AppendMessage instead).
// Verifies the session exists; touches updated_at like AppendMessage.
func (s *SQLiteStore) ReplaceMessages(sessionID string, msgs []wingmodels.Message) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM sessions WHERE id = ?`, sessionID).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("session not found: %s", sessionID)
		}
		return err
	}

	if _, err := tx.Exec(`DELETE FROM messages WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("delete prior messages: %w", err)
	}
	if err := writeMessages(tx, sessionID, msgs); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE sessions SET updated_at = ? WHERE id = ?`,
		Now(), sessionID,
	); err != nil {
		return fmt.Errorf("bump session updated_at: %w", err)
	}
	return tx.Commit()
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

// txExecer is the subset of *sql.Tx and *sql.DB we need for inserting
// messages and parts. Lets writeMessages run inside any transaction.
type txExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// writeMessages inserts every message + part for a session, in order.
// Caller is responsible for clearing prior rows if doing a replace.
func writeMessages(tx txExecer, sessionID string, msgs []wingmodels.Message) error {
	now := Now()
	for i, msg := range msgs {
		msgID := NewID(PrefixMessage)
		if _, err := tx.Exec(`
			INSERT INTO messages (id, session_id, idx, role, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, msgID, sessionID, i, string(msg.Role), now); err != nil {
			return fmt.Errorf("insert message %d: %w", i, err)
		}
		for j, part := range msg.Content {
			payload, err := wingmodels.MarshalPart(part)
			if err != nil {
				return fmt.Errorf("marshal part %d/%d: %w", i, j, err)
			}
			if _, err := tx.Exec(`
				INSERT INTO parts (id, message_id, idx, kind, payload_json)
				VALUES (?, ?, ?, ?, ?)
			`, NewID(PrefixPart), msgID, j, part.Type(), string(payload)); err != nil {
				return fmt.Errorf("insert part %d/%d: %w", i, j, err)
			}
		}
	}
	return nil
}

// readMessages reads every message + part for a session, in order, and
// reconstructs the wingmodels.Message slice. Empty session = empty slice
// (never nil; callers check len).
func readMessages(db *sql.DB, sessionID string) ([]wingmodels.Message, error) {
	rows, err := db.Query(`
		SELECT id, idx, role FROM messages WHERE session_id = ? ORDER BY idx ASC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}

	type msgRow struct {
		id   string
		role string
	}
	var meta []msgRow
	for rows.Next() {
		var m msgRow
		var idx int
		if err := rows.Scan(&m.id, &idx, &m.role); err != nil {
			rows.Close()
			return nil, err
		}
		meta = append(meta, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]wingmodels.Message, 0, len(meta))
	for _, m := range meta {
		parts, err := readParts(db, m.id)
		if err != nil {
			return nil, err
		}
		out = append(out, wingmodels.Message{
			Role:    wingmodels.Role(m.role),
			Content: parts,
		})
	}
	return out, nil
}

// readParts reads every part for a message, in order, and decodes them
// via wingmodels.UnmarshalPart.
func readParts(db *sql.DB, messageID string) (wingmodels.Content, error) {
	rows, err := db.Query(`
		SELECT payload_json FROM parts WHERE message_id = ? ORDER BY idx ASC
	`, messageID)
	if err != nil {
		return nil, fmt.Errorf("query parts: %w", err)
	}
	defer rows.Close()

	var parts wingmodels.Content
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		p, err := wingmodels.UnmarshalPart([]byte(payload))
		if err != nil {
			return nil, fmt.Errorf("unmarshal part: %w", err)
		}
		parts = append(parts, p)
	}
	return parts, rows.Err()
}
