package storage

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/oklog/ulid/v2"
	_ "modernc.org/sqlite"

	"github.com/chaserensberger/wingman/models"
)

const schema = `
CREATE TABLE IF NOT EXISTS agents (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	instructions TEXT,
	tools TEXT,
	model TEXT,
	options TEXT,
	output_schema TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	work_dir TEXT,
	history TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS fleets (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	agent_id TEXT NOT NULL,
	worker_count INTEGER NOT NULL,
	work_dir TEXT,
	status TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS formations (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	work_dir TEXT,
	roles TEXT NOT NULL,
	edges TEXT,
	status TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS auth (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	providers TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
`

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "wingman", "wingman.db"), nil
}

func NewID() string {
	entropy := ulid.Monotonic(rand.Reader, 0)
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func (s *SQLiteStore) CreateAgent(agent *Agent) error {
	if agent.ID == "" {
		agent.ID = NewID()
	}
	now := Now()
	agent.CreatedAt = now
	agent.UpdatedAt = now

	tools, err := json.Marshal(agent.Tools)
	if err != nil {
		return err
	}

	var optionsJSON *string
	if agent.Options != nil {
		b, err := json.Marshal(agent.Options)
		if err != nil {
			return err
		}
		s := string(b)
		optionsJSON = &s
	}

	var outputSchemaJSON *string
	if agent.OutputSchema != nil {
		b, err := json.Marshal(agent.OutputSchema)
		if err != nil {
			return err
		}
		s := string(b)
		outputSchemaJSON = &s
	}

	_, err = s.db.Exec(`
		INSERT INTO agents (id, name, instructions, tools, model, options, output_schema, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, agent.ID, agent.Name, agent.Instructions, string(tools), agent.Model, optionsJSON, outputSchemaJSON, agent.CreatedAt, agent.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetAgent(id string) (*Agent, error) {
	var agent Agent
	var toolsJSON string
	var optionsJSON sql.NullString
	var outputSchemaJSON sql.NullString

	err := s.db.QueryRow(`
		SELECT id, name, instructions, tools, model, options, output_schema, created_at, updated_at
		FROM agents WHERE id = ?
	`, id).Scan(&agent.ID, &agent.Name, &agent.Instructions, &toolsJSON, &agent.Model, &optionsJSON, &outputSchemaJSON, &agent.CreatedAt, &agent.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	if toolsJSON != "" {
		if err := json.Unmarshal([]byte(toolsJSON), &agent.Tools); err != nil {
			return nil, err
		}
	}

	if optionsJSON.Valid && optionsJSON.String != "" {
		if err := json.Unmarshal([]byte(optionsJSON.String), &agent.Options); err != nil {
			return nil, err
		}
	}

	if outputSchemaJSON.Valid && outputSchemaJSON.String != "" {
		if err := json.Unmarshal([]byte(outputSchemaJSON.String), &agent.OutputSchema); err != nil {
			return nil, err
		}
	}

	return &agent, nil
}

func (s *SQLiteStore) ListAgents() ([]*Agent, error) {
	rows, err := s.db.Query(`
		SELECT id, name, instructions, tools, model, options, output_schema, created_at, updated_at
		FROM agents ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []*Agent
	for rows.Next() {
		var agent Agent
		var toolsJSON string
		var optionsJSON sql.NullString
		var outputSchemaJSON sql.NullString

		if err := rows.Scan(&agent.ID, &agent.Name, &agent.Instructions, &toolsJSON, &agent.Model, &optionsJSON, &outputSchemaJSON, &agent.CreatedAt, &agent.UpdatedAt); err != nil {
			return nil, err
		}

		if toolsJSON != "" {
			if err := json.Unmarshal([]byte(toolsJSON), &agent.Tools); err != nil {
				return nil, err
			}
		}

		if optionsJSON.Valid && optionsJSON.String != "" {
			if err := json.Unmarshal([]byte(optionsJSON.String), &agent.Options); err != nil {
				return nil, err
			}
		}

		if outputSchemaJSON.Valid && outputSchemaJSON.String != "" {
			if err := json.Unmarshal([]byte(outputSchemaJSON.String), &agent.OutputSchema); err != nil {
				return nil, err
			}
		}

		agents = append(agents, &agent)
	}

	return agents, rows.Err()
}

func (s *SQLiteStore) UpdateAgent(agent *Agent) error {
	agent.UpdatedAt = Now()

	tools, err := json.Marshal(agent.Tools)
	if err != nil {
		return err
	}

	var optionsJSON *string
	if agent.Options != nil {
		b, err := json.Marshal(agent.Options)
		if err != nil {
			return err
		}
		s := string(b)
		optionsJSON = &s
	}

	var outputSchemaJSON *string
	if agent.OutputSchema != nil {
		b, err := json.Marshal(agent.OutputSchema)
		if err != nil {
			return err
		}
		s := string(b)
		outputSchemaJSON = &s
	}

	result, err := s.db.Exec(`
		UPDATE agents SET name = ?, instructions = ?, tools = ?, model = ?, options = ?, output_schema = ?, updated_at = ?
		WHERE id = ?
	`, agent.Name, agent.Instructions, string(tools), agent.Model, optionsJSON, outputSchemaJSON, agent.UpdatedAt, agent.ID)

	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("agent not found: %s", agent.ID)
	}
	return nil
}

func (s *SQLiteStore) DeleteAgent(id string) error {
	result, err := s.db.Exec(`DELETE FROM agents WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("agent not found: %s", id)
	}
	return nil
}

func (s *SQLiteStore) CreateSession(session *Session) error {
	if session.ID == "" {
		session.ID = NewID()
	}
	now := Now()
	session.CreatedAt = now
	session.UpdatedAt = now

	if session.History == nil {
		session.History = []models.WingmanMessage{}
	}

	history, err := json.Marshal(session.History)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
		INSERT INTO sessions (id, work_dir, history, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, session.ID, session.WorkDir, string(history), session.CreatedAt, session.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetSession(id string) (*Session, error) {
	var session Session
	var historyJSON string

	err := s.db.QueryRow(`
		SELECT id, work_dir, history, created_at, updated_at
		FROM sessions WHERE id = ?
	`, id).Scan(&session.ID, &session.WorkDir, &historyJSON, &session.CreatedAt, &session.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	if historyJSON != "" {
		if err := json.Unmarshal([]byte(historyJSON), &session.History); err != nil {
			return nil, err
		}
	}

	return &session, nil
}

func (s *SQLiteStore) ListSessions() ([]*Session, error) {
	rows, err := s.db.Query(`
		SELECT id, work_dir, history, created_at, updated_at
		FROM sessions ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var session Session
		var historyJSON string

		if err := rows.Scan(&session.ID, &session.WorkDir, &historyJSON, &session.CreatedAt, &session.UpdatedAt); err != nil {
			return nil, err
		}

		if historyJSON != "" {
			if err := json.Unmarshal([]byte(historyJSON), &session.History); err != nil {
				return nil, err
			}
		}

		sessions = append(sessions, &session)
	}

	return sessions, rows.Err()
}

func (s *SQLiteStore) UpdateSession(session *Session) error {
	session.UpdatedAt = Now()

	history, err := json.Marshal(session.History)
	if err != nil {
		return err
	}

	result, err := s.db.Exec(`
		UPDATE sessions SET work_dir = ?, history = ?, updated_at = ?
		WHERE id = ?
	`, session.WorkDir, string(history), session.UpdatedAt, session.ID)

	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("session not found: %s", session.ID)
	}
	return nil
}

func (s *SQLiteStore) DeleteSession(id string) error {
	result, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("session not found: %s", id)
	}
	return nil
}

func (s *SQLiteStore) CreateFleet(fleet *Fleet) error {
	if fleet.ID == "" {
		fleet.ID = NewID()
	}
	now := Now()
	fleet.CreatedAt = now
	fleet.UpdatedAt = now
	fleet.Status = FleetStatusStopped

	_, err := s.db.Exec(`
		INSERT INTO fleets (id, name, agent_id, worker_count, work_dir, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, fleet.ID, fleet.Name, fleet.AgentID, fleet.WorkerCount, fleet.WorkDir, fleet.Status, fleet.CreatedAt, fleet.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create fleet: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetFleet(id string) (*Fleet, error) {
	var fleet Fleet

	err := s.db.QueryRow(`
		SELECT id, name, agent_id, worker_count, work_dir, status, created_at, updated_at
		FROM fleets WHERE id = ?
	`, id).Scan(&fleet.ID, &fleet.Name, &fleet.AgentID, &fleet.WorkerCount, &fleet.WorkDir, &fleet.Status, &fleet.CreatedAt, &fleet.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("fleet not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	return &fleet, nil
}

func (s *SQLiteStore) ListFleets() ([]*Fleet, error) {
	rows, err := s.db.Query(`
		SELECT id, name, agent_id, worker_count, work_dir, status, created_at, updated_at
		FROM fleets ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fleets []*Fleet
	for rows.Next() {
		var fleet Fleet

		if err := rows.Scan(&fleet.ID, &fleet.Name, &fleet.AgentID, &fleet.WorkerCount, &fleet.WorkDir, &fleet.Status, &fleet.CreatedAt, &fleet.UpdatedAt); err != nil {
			return nil, err
		}

		fleets = append(fleets, &fleet)
	}

	return fleets, rows.Err()
}

func (s *SQLiteStore) UpdateFleet(fleet *Fleet) error {
	fleet.UpdatedAt = Now()

	result, err := s.db.Exec(`
		UPDATE fleets SET name = ?, agent_id = ?, worker_count = ?, work_dir = ?, status = ?, updated_at = ?
		WHERE id = ?
	`, fleet.Name, fleet.AgentID, fleet.WorkerCount, fleet.WorkDir, fleet.Status, fleet.UpdatedAt, fleet.ID)

	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("fleet not found: %s", fleet.ID)
	}
	return nil
}

func (s *SQLiteStore) DeleteFleet(id string) error {
	result, err := s.db.Exec(`DELETE FROM fleets WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("fleet not found: %s", id)
	}
	return nil
}

func (s *SQLiteStore) CreateFormation(formation *Formation) error {
	if formation.ID == "" {
		formation.ID = NewID()
	}
	now := Now()
	formation.CreatedAt = now
	formation.UpdatedAt = now
	formation.Status = FormationStatusStopped

	roles, err := json.Marshal(formation.Roles)
	if err != nil {
		return err
	}

	edges, err := json.Marshal(formation.Edges)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
		INSERT INTO formations (id, name, work_dir, roles, edges, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, formation.ID, formation.Name, formation.WorkDir, string(roles), string(edges), formation.Status, formation.CreatedAt, formation.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create formation: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetFormation(id string) (*Formation, error) {
	var formation Formation
	var rolesJSON, edgesJSON string

	err := s.db.QueryRow(`
		SELECT id, name, work_dir, roles, edges, status, created_at, updated_at
		FROM formations WHERE id = ?
	`, id).Scan(&formation.ID, &formation.Name, &formation.WorkDir, &rolesJSON, &edgesJSON, &formation.Status, &formation.CreatedAt, &formation.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("formation not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	if rolesJSON != "" {
		if err := json.Unmarshal([]byte(rolesJSON), &formation.Roles); err != nil {
			return nil, err
		}
	}

	if edgesJSON != "" {
		if err := json.Unmarshal([]byte(edgesJSON), &formation.Edges); err != nil {
			return nil, err
		}
	}

	return &formation, nil
}

func (s *SQLiteStore) ListFormations() ([]*Formation, error) {
	rows, err := s.db.Query(`
		SELECT id, name, work_dir, roles, edges, status, created_at, updated_at
		FROM formations ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var formations []*Formation
	for rows.Next() {
		var formation Formation
		var rolesJSON, edgesJSON string

		if err := rows.Scan(&formation.ID, &formation.Name, &formation.WorkDir, &rolesJSON, &edgesJSON, &formation.Status, &formation.CreatedAt, &formation.UpdatedAt); err != nil {
			return nil, err
		}

		if rolesJSON != "" {
			if err := json.Unmarshal([]byte(rolesJSON), &formation.Roles); err != nil {
				return nil, err
			}
		}

		if edgesJSON != "" {
			if err := json.Unmarshal([]byte(edgesJSON), &formation.Edges); err != nil {
				return nil, err
			}
		}

		formations = append(formations, &formation)
	}

	return formations, rows.Err()
}

func (s *SQLiteStore) UpdateFormation(formation *Formation) error {
	formation.UpdatedAt = Now()

	roles, err := json.Marshal(formation.Roles)
	if err != nil {
		return err
	}

	edges, err := json.Marshal(formation.Edges)
	if err != nil {
		return err
	}

	result, err := s.db.Exec(`
		UPDATE formations SET name = ?, work_dir = ?, roles = ?, edges = ?, status = ?, updated_at = ?
		WHERE id = ?
	`, formation.Name, formation.WorkDir, string(roles), string(edges), formation.Status, formation.UpdatedAt, formation.ID)

	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("formation not found: %s", formation.ID)
	}
	return nil
}

func (s *SQLiteStore) DeleteFormation(id string) error {
	result, err := s.db.Exec(`DELETE FROM formations WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("formation not found: %s", id)
	}
	return nil
}

func (s *SQLiteStore) GetAuth() (*Auth, error) {
	var auth Auth
	var providersJSON string

	err := s.db.QueryRow(`SELECT providers, updated_at FROM auth WHERE id = 1`).Scan(&providersJSON, &auth.UpdatedAt)

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

func (s *SQLiteStore) SetAuth(auth *Auth) error {
	auth.UpdatedAt = Now()

	providers, err := json.Marshal(auth.Providers)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
		INSERT INTO auth (id, providers, updated_at) VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET providers = ?, updated_at = ?
	`, string(providers), auth.UpdatedAt, string(providers), auth.UpdatedAt)

	return err
}
