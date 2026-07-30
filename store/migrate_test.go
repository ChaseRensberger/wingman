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
