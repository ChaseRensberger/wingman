package store

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// migrationsFS embeds the .sql files under ./migrations. Files are named
// NNNN_name.sql (e.g., 0001_init.sql). The numeric prefix is the
// migration version; it must be a strictly increasing integer with no
// gaps. The runner refuses to apply gaps.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationsTable is the schema-tracking table. We create it lazily on
// first run; it is not itself defined in a migration file (chicken/egg).
const migrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    checksum   TEXT NOT NULL,
    applied_at TEXT NOT NULL
);`

// migration is one parsed entry from migrationsFS.
type migration struct {
	version  int
	name     string
	sql      string
	checksum string
}

// loadMigrations reads every embedded .sql file and returns them sorted
// by version. Returns an error if any filename fails to parse, or if
// versions have gaps / duplicates.
func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	out := make([]migration, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		// Filename: NNNN_some_name.sql
		stem := strings.TrimSuffix(e.Name(), ".sql")
		idx := strings.IndexByte(stem, '_')
		if idx <= 0 {
			return nil, fmt.Errorf("migration %q: expected NNNN_name.sql", e.Name())
		}
		var v int
		if _, err := fmt.Sscanf(stem[:idx], "%d", &v); err != nil {
			return nil, fmt.Errorf("migration %q: bad version: %w", e.Name(), err)
		}
		body, err := fs.ReadFile(migrationsFS, "migrations/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", e.Name(), err)
		}
		sql := string(body)
		out = append(out, migration{version: v, name: stem[idx+1:], sql: sql, checksum: migrationChecksum(sql)})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })

	// Validate contiguity. Gaps usually mean a migration was deleted; we
	// fail loudly rather than skip silently.
	for i, m := range out {
		want := i + 1
		if m.version != want {
			return nil, fmt.Errorf("migration version gap: expected %d, got %d (%s)", want, m.version, m.name)
		}
	}
	return out, nil
}

func migrationChecksum(sql string) string {
	sum := sha256.Sum256([]byte(sql))
	return hex.EncodeToString(sum[:])
}

// runMigrations validates the applied migration history, then applies every
// remaining embedded migration. Each migration runs in its own transaction;
// a partial failure rolls back that one migration but keeps prior ones.
// Subsequent runs pick up where we left off.
//
// Idempotent: running this on an up-to-date DB makes no schema changes.
func runMigrations(db *sql.DB) error {
	if _, err := db.Exec(migrationsTable); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	if err := ensureMigrationChecksums(db); err != nil {
		return err
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	applied, err := validateAppliedMigrations(db, migrations)
	if err != nil {
		return err
	}

	for _, m := range migrations[applied:] {
		if err := applyMigration(db, m); err != nil {
			return fmt.Errorf("apply migration %d (%s): %w", m.version, m.name, err)
		}
	}
	return nil
}

func ensureMigrationChecksums(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(schema_migrations)`)
	if err != nil {
		return fmt.Errorf("inspect schema_migrations: %w", err)
	}

	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read schema_migrations columns: %w", err)
		}
		if name == "checksum" {
			if err := rows.Close(); err != nil {
				return fmt.Errorf("close schema_migrations columns: %w", err)
			}
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("read schema_migrations columns: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close schema_migrations columns: %w", err)
	}
	if _, err := db.Exec(`ALTER TABLE schema_migrations ADD COLUMN checksum TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add schema_migrations checksum: %w", err)
	}
	return nil
}

func validateAppliedMigrations(db *sql.DB, migrations []migration) (int, error) {
	rows, err := db.Query(`SELECT version, name, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return 0, fmt.Errorf("read schema_migrations: %w", err)
	}

	type appliedMigration struct {
		version  int
		name     string
		checksum string
	}
	var history []appliedMigration
	for rows.Next() {
		var applied appliedMigration
		if err := rows.Scan(&applied.version, &applied.name, &applied.checksum); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("read schema_migrations row: %w", err)
		}
		history = append(history, applied)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, fmt.Errorf("read schema_migrations: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close schema_migrations: %w", err)
	}

	for applied, actual := range history {
		if applied == len(migrations) {
			return 0, fmt.Errorf("migration history has unknown version %d (%s)", actual.version, actual.name)
		}

		expected := migrations[applied]
		if actual.version != expected.version {
			return 0, fmt.Errorf("migration history gap: expected version %d, got %d (%s)", expected.version, actual.version, actual.name)
		}
		if actual.name != expected.name {
			return 0, fmt.Errorf("migration history name mismatch for version %d: got %q, want %q", actual.version, actual.name, expected.name)
		}
		if actual.checksum == "" {
			// Databases created before checksums existed are trusted once after
			// their version and names have been validated against this binary.
			if _, err := db.Exec(`UPDATE schema_migrations SET checksum = ? WHERE version = ?`, expected.checksum, actual.version); err != nil {
				return 0, fmt.Errorf("backfill migration checksum for version %d: %w", actual.version, err)
			}
		} else if actual.checksum != expected.checksum {
			return 0, fmt.Errorf("migration history checksum mismatch for version %d (%s)", actual.version, actual.name)
		}
	}
	return len(history), nil
}

// applyMigration runs one migration's SQL inside a transaction and
// records it in schema_migrations on success.
func applyMigration(db *sql.DB, m migration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op if Commit succeeds

	if _, err := tx.Exec(m.sql); err != nil {
		return fmt.Errorf("exec sql: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`,
		m.version, m.name, m.checksum, Now(),
	); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}
	return tx.Commit()
}
