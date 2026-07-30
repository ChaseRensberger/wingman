package store

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunMigrationsRecordsChecksums(t *testing.T) {
	db := testMigrationDB(t)
	if err := runMigrations(db); err != nil {
		t.Fatal(err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE checksum <> ''`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != len(migrations) {
		t.Fatalf("checksummed migrations = %d, want %d", count, len(migrations))
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

	err := runMigrations(db)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
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

	err := runMigrations(db)
	if err == nil || !strings.Contains(err.Error(), "name mismatch") {
		t.Fatalf("runMigrations error = %v, want name mismatch", err)
	}
}

func TestRunMigrationsRejectsGappedHistory(t *testing.T) {
	db := testMigrationDB(t)
	if err := runMigrations(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version = 2`); err != nil {
		t.Fatal(err)
	}

	err := runMigrations(db)
	if err == nil || !strings.Contains(err.Error(), "history gap") {
		t.Fatalf("runMigrations error = %v, want migration history gap", err)
	}
}

func TestRunMigrationsBackfillsLegacyChecksums(t *testing.T) {
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
	for _, migration := range migrations {
		if _, err := db.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`, migration.version, migration.name, Now()); err != nil {
			t.Fatal(err)
		}
	}

	if err := runMigrations(db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE checksum <> ''`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != len(migrations) {
		t.Fatalf("checksummed migrations = %d, want %d", count, len(migrations))
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

func TestAggregateEventMigrationBackfillsSessions(t *testing.T) {
	db := testMigrationDB(t)
	if _, err := db.Exec(migrationsTable); err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) < 6 {
		t.Fatalf("migrations = %d, want at least 6", len(migrations))
	}
	for _, migration := range migrations[:5] {
		if err := applyMigration(db, migration); err != nil {
			t.Fatal(err)
		}
	}
	createdAt := "2026-07-30T12:00:00Z"
	if _, err := db.Exec(`
		INSERT INTO sessions (id, title, work_dir, created_at, updated_at)
		VALUES ('ses_legacy', 'Legacy', '/tmp/legacy', ?, ?)
	`, createdAt, createdAt); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO sessions (id, title, created_at, updated_at)
		VALUES ('old_legacy', 'Noncanonical legacy ID', ?, ?)
	`, createdAt, createdAt); err != nil {
		t.Fatal(err)
	}
	if err := applyMigration(db, migrations[5]); err != nil {
		t.Fatal(err)
	}

	var projectionVersion int64
	if err := db.QueryRow(`SELECT aggregate_version FROM sessions WHERE id = 'ses_legacy'`).Scan(&projectionVersion); err != nil {
		t.Fatal(err)
	}
	if projectionVersion != 1 {
		t.Fatalf("projection version = %d, want 1", projectionVersion)
	}
	var eventType string
	var eventVersion int64
	var payload string
	if err := db.QueryRow(`
		SELECT event_type, version, payload_json
		FROM aggregate_events
		WHERE aggregate_type = 'session' AND aggregate_id = 'ses_legacy'
	`).Scan(&eventType, &eventVersion, &payload); err != nil {
		t.Fatal(err)
	}
	if eventType != EventSessionCreated || eventVersion != 1 {
		t.Fatalf("event = %s version %d", eventType, eventVersion)
	}
	for _, want := range []string{`"id":"ses_legacy"`, `"title":"Legacy"`, `"work_dir":"/tmp/legacy"`} {
		if !strings.Contains(payload, want) {
			t.Fatalf("payload %s does not contain %s", payload, want)
		}
	}
	var eventCount, distinctIDCount int
	if err := db.QueryRow(`SELECT COUNT(*), COUNT(DISTINCT id) FROM aggregate_events`).Scan(&eventCount, &distinctIDCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 2 || distinctIDCount != 2 {
		t.Fatalf("events = %d, distinct IDs = %d; want 2 each", eventCount, distinctIDCount)
	}
}

func TestSessionRunAdmissionMigrationBackfillsAggregateEvents(t *testing.T) {
	db := testMigrationDB(t)
	if _, err := db.Exec(migrationsTable); err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations[:6] {
		if err := applyMigration(db, migration); err != nil {
			t.Fatal(err)
		}
	}
	created := "2026-07-30T12:00:00Z"
	if _, err := db.Exec(`INSERT INTO sessions (id, title, work_dir, created_at, updated_at, aggregate_version) VALUES ('ses_legacy_run', '', '/legacy', ?, ?, 1)`, created, created); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO aggregate_events (id, aggregate_type, aggregate_id, version, event_type, schema_version, payload_json, created_at) VALUES ('evt_legacy_created', 'session', 'ses_legacy_run', 1, 'session.created', 1, '{"id":"ses_legacy_run"}', ?), ('evt_legacy_extra', 'session', 'ses_legacy_run', 2, 'session.renamed', 1, '{}', ?)`, created, created); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sessions SET aggregate_version = 2 WHERE id = 'ses_legacy_run'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO session_runs (id, session_id, sequence, status, message, agent_json, created_at, updated_at) VALUES ('run_legacy', 'ses_legacy_run', 1, 'queued', 'hello', '{"id":"agt_legacy"}', ?, ?), ('run_legacy_2', 'ses_legacy_run', 2, 'completed', 'again', '{"id":"agt_legacy"}', ?, ?)`, created, created, created, created); err != nil {
		t.Fatal(err)
	}
	if err := applyMigration(db, migrations[6]); err != nil {
		t.Fatal(err)
	}
	var version, admittedVersion int64
	var requestID, requestHash string
	if err := db.QueryRow(`SELECT aggregate_version FROM sessions WHERE id = 'ses_legacy_run'`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT admitted_version, request_id, request_hash FROM session_runs WHERE id = 'run_legacy'`).Scan(&admittedVersion, &requestID, &requestHash); err != nil {
		t.Fatal(err)
	}
	if version != 4 || admittedVersion != 3 || requestID != "" || requestHash != "" {
		t.Fatalf("version=%d admitted=%d request=%q hash=%q", version, admittedVersion, requestID, requestHash)
	}
	var secondVersion int64
	if err := db.QueryRow(`SELECT admitted_version FROM session_runs WHERE id = 'run_legacy_2'`).Scan(&secondVersion); err != nil {
		t.Fatal(err)
	}
	if secondVersion != 4 {
		t.Fatalf("second admitted version = %d, want 4", secondVersion)
	}
	var admissionEvents int
	if err := db.QueryRow(`SELECT COUNT(*) FROM aggregate_events WHERE aggregate_id = 'ses_legacy_run' AND event_type = 'session.run.admitted'`).Scan(&admissionEvents); err != nil {
		t.Fatal(err)
	}
	if admissionEvents != 2 {
		t.Fatalf("admission events = %d, want 2", admissionEvents)
	}
	var payload string
	if err := db.QueryRow(`SELECT payload_json FROM aggregate_events WHERE run_id = 'run_legacy'`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `"work_dir":"/legacy"`) || !strings.Contains(payload, `"message":"hello"`) {
		t.Fatalf("payload = %s", payload)
	}
}

func TestModelCallAttemptsMigrationPreservesLegacyCalls(t *testing.T) {
	db := testMigrationDB(t)
	if _, err := db.Exec(migrationsTable); err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations[:7] {
		if err := applyMigration(db, migration); err != nil {
			t.Fatal(err)
		}
	}
	created := "2026-07-30T12:00:00Z"
	if _, err := db.Exec(`INSERT INTO sessions (id, title, created_at, updated_at, aggregate_version) VALUES ('ses_legacy_calls', '', ?, ?, 1)`, created, created); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO model_calls (id, session_id, step, attempt, status, started_at, created_at, updated_at)
		VALUES ('mcl_legacy_one', 'ses_legacy_calls', 1, 1, 'completed', ?, ?, ?), ('mcl_legacy_two', 'ses_legacy_calls', 2, 1, 'completed', ?, ?, ?)
	`, created, created, created, created, created, created); err != nil {
		t.Fatal(err)
	}
	if err := applyMigration(db, migrations[7]); err != nil {
		t.Fatal(err)
	}
	var calls, runs int
	if err := db.QueryRow(`SELECT COUNT(*), COUNT(run_id) FROM model_calls WHERE session_id = 'ses_legacy_calls'`).Scan(&calls, &runs); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || runs != 0 {
		t.Fatalf("legacy calls=%d run IDs=%d, want 2 and 0", calls, runs)
	}
	if _, err := db.Exec(`INSERT INTO session_runs (id, session_id, sequence, status, message, agent_json, created_at, updated_at) VALUES ('run_new', 'ses_legacy_calls', 1, 'queued', '', '{}', ?, ?)`, created, created); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO model_calls (id, session_id, run_id, step, attempt, status, provider_request_id, started_at, created_at, updated_at) VALUES ('mcl_new', 'ses_legacy_calls', 'run_new', 1, 1, 'started', 'request_new', ?, ?, ?)`, created, created, created); err != nil {
		t.Fatal(err)
	}
	var providerRequestID string
	if err := db.QueryRow(`SELECT provider_request_id FROM model_calls WHERE id = 'mcl_new'`).Scan(&providerRequestID); err != nil || providerRequestID != "request_new" {
		t.Fatalf("provider request ID=%q error=%v", providerRequestID, err)
	}
	if _, err := db.Exec(`INSERT INTO model_calls (id, session_id, run_id, step, attempt, status, started_at, created_at, updated_at) VALUES ('mcl_duplicate', 'ses_legacy_calls', 'run_new', 1, 1, 'started', ?, ?, ?)`, created, created, created); err == nil {
		t.Fatal("duplicate run attempt insert succeeded")
	}
	for _, index := range []string{"idx_model_calls_session_started_at", "idx_model_calls_assistant_message", "idx_model_calls_session_status"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, index).Scan(&count); err != nil || count != 1 {
			t.Fatalf("index %s count=%d error=%v", index, count, err)
		}
	}
	rows, err := db.Query(`SELECT name FROM pragma_index_info('idx_model_calls_session_started_at') ORDER BY seqno`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil || strings.Join(columns, ",") != "session_id,started_at,id" {
		t.Fatalf("chronology index columns=%v error=%v", columns, err)
	}
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
