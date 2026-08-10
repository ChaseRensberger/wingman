package store

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunMigrationsCreatesCanonicalSchema(t *testing.T) {
	db := testMigrationDB(t)
	if err := runMigrations(db); err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 1 || migrations[0].version != 1 || migrations[0].name != "init" {
		t.Fatalf("migrations = %#v, want 0001_init", migrations)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 1 AND name = 'init' AND checksum <> ''`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("recorded initial migration = %d, want 1", count)
	}
	for table, columns := range map[string][]string{
		"agents":              {"permissions_json"},
		"sessions":            {"aggregate_version"},
		"messages":            {"run_id"},
		"session_runs":        {"request_id", "request_hash", "admitted_version", "work_dir", "workspace_id", "client_id", "error_type"},
		"model_calls":         {"run_id", "provider_request_id"},
		"tool_uses":           {"run_id", "model_call_id", "assistant_message_id", "part_id", "ordinal", "call_id", "structured_json", "proposed_at"},
		"permission_requests": {"session_id", "run_id", "tool_use_id", "resources_json", "resolved_at"},
		"permission_grants":   {"session_id", "action", "resource"},
		"session_events":      {"schema_version"},
		"aggregate_events":    {"global_sequence", "schema_version", "causation_id", "correlation_id", "client_id", "run_id"},
	} {
		for _, column := range columns {
			if !schemaHasColumn(t, db, table, column) {
				t.Fatalf("%s.%s is missing", table, column)
			}
		}
	}
	for _, table := range []string{"agents", "clients", "workspaces", "sessions", "messages", "parts", "session_runs", "model_calls", "tool_uses", "permission_requests", "permission_grants", "session_events", "aggregate_events", "auth"} {
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("table %s count=%d error=%v", table, count, err)
		}
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name LIKE '%legacy%'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("legacy schema objects = %d, want 0", count)
	}
	for _, index := range []string{
		"idx_session_runs_session_request_id",
		"idx_session_runs_one_running_per_session",
		"idx_model_calls_session_started_at",
		"idx_tool_uses_run_step_ordinal",
		"idx_permission_requests_session_created",
		"idx_permission_grants_session",
		"idx_aggregate_events_stream",
		"idx_session_events_session_seq",
	} {
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, index).Scan(&count); err != nil || count != 1 {
			t.Fatalf("index %s count=%d error=%v", index, count, err)
		}
	}
}

func TestPermissionRequestSchemaRejectsInvalidValues(t *testing.T) {
	db := testMigrationDB(t)
	if err := runMigrations(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sessions (id, created_at, updated_at) VALUES ('ses_permissions', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	insert := func(id, status, response, resources string) error {
		_, err := db.Exec(`INSERT INTO permission_requests (id, session_id, action, resources_json, status, response, created_at, updated_at) VALUES (?, 'ses_permissions', 'shell.exec', ?, ?, ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, id, resources, status, response)
		return err
	}
	if err := insert("prq_valid", "pending", "", `["pwd"]`); err != nil {
		t.Fatalf("valid permission request: %v", err)
	}
	for _, test := range []struct {
		id, status, response, resources string
	}{
		{"prq_bad_status", "unknown", "", `["pwd"]`},
		{"prq_bad_response", "pending", "unknown", `["pwd"]`},
		{"prq_pending_once", "pending", "once", `["pwd"]`},
		{"prq_approved_empty", "approved", "", `["pwd"]`},
		{"prq_rejected_once", "rejected", "once", `["pwd"]`},
		{"prq_timeout_reject", "timed_out", "reject", `["pwd"]`},
		{"prq_object_resources", "pending", "", `{}`},
		{"prq_empty_resources", "pending", "", `[]`},
	} {
		if err := insert(test.id, test.status, test.response, test.resources); err == nil {
			t.Fatalf("invalid permission request %#v was accepted", test)
		}
	}
}

func TestRunMigrationsRejectsTamperedChecksum(t *testing.T) {
	db := testMigrationDB(t)
	if err := runMigrations(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE schema_migrations SET checksum = 'tampered' WHERE version = 1`); err != nil {
		t.Fatal(err)
	}
	if err := runMigrations(db); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("runMigrations error = %v, want checksum mismatch", err)
	}
}

func TestRunMigrationsRejectsRenamedHistory(t *testing.T) {
	db := testMigrationDB(t)
	if err := runMigrations(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE schema_migrations SET name = 'renamed' WHERE version = 1`); err != nil {
		t.Fatal(err)
	}
	if err := runMigrations(db); err == nil || !strings.Contains(err.Error(), "name mismatch") {
		t.Fatalf("runMigrations error = %v, want name mismatch", err)
	}
}

func TestRunMigrationsRejectsGappedHistory(t *testing.T) {
	db := testMigrationDB(t)
	if _, err := db.Exec(migrationsTable); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (2, 'future', 'checksum', ?)`, Now()); err != nil {
		t.Fatal(err)
	}
	if err := runMigrations(db); err == nil || !strings.Contains(err.Error(), "history gap") {
		t.Fatalf("runMigrations error = %v, want history gap", err)
	}
}

func TestRunMigrationsRejectsUnknownHistory(t *testing.T) {
	db := testMigrationDB(t)
	if err := runMigrations(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (3, 'removed', 'checksum', ?)`, Now()); err != nil {
		t.Fatal(err)
	}
	if err := runMigrations(db); err == nil || !strings.Contains(err.Error(), "unknown version") {
		t.Fatalf("runMigrations error = %v, want unknown version", err)
	}
}

func TestRunMigrationsBackfillsChecksumColumn(t *testing.T) {
	db := testMigrationDB(t)
	if _, err := db.Exec(`
CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at TEXT NOT NULL
)`); err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(migrations[0].sql); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (1, 'init', ?)`, Now()); err != nil {
		t.Fatal(err)
	}
	if err := runMigrations(db); err != nil {
		t.Fatal(err)
	}
	var checksum string
	if err := db.QueryRow(`SELECT checksum FROM schema_migrations WHERE version = 1`).Scan(&checksum); err != nil {
		t.Fatal(err)
	}
	if checksum == "" {
		t.Fatal("checksum was not backfilled")
	}
}

func TestApplyMigrationRollsBackOnFailure(t *testing.T) {
	db := testMigrationDB(t)
	if _, err := db.Exec(migrationsTable); err != nil {
		t.Fatal(err)
	}
	err := applyMigration(db, migration{version: 1, name: "invalid", sql: `CREATE TABLE transient (id INTEGER); INVALID SQL;`})
	if err == nil {
		t.Fatal("applyMigration succeeded, want error")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("recorded migrations = %d, want 0", count)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'transient'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("transient table exists after failed migration")
	}
}

func schemaHasColumn(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return false
}

func testMigrationDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}
