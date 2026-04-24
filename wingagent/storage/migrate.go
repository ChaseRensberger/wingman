package storage

import (
	"database/sql"
	"embed"
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
    applied_at TEXT NOT NULL
);`

// migration is one parsed entry from migrationsFS.
type migration struct {
	version int
	name    string
	sql     string
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
		base := strings.TrimSuffix(e.Name(), ".sql")
		idx := strings.IndexByte(base, '_')
		if idx <= 0 {
			return nil, fmt.Errorf("migration %q: expected NNNN_name.sql", e.Name())
		}
		var v int
		if _, err := fmt.Sscanf(base[:idx], "%d", &v); err != nil {
			return nil, fmt.Errorf("migration %q: bad version: %w", e.Name(), err)
		}
		body, err := fs.ReadFile(migrationsFS, "migrations/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", e.Name(), err)
		}
		out = append(out, migration{version: v, name: base[idx+1:], sql: string(body)})
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

// runMigrations applies every embedded migration whose version is higher
// than the highest version recorded in schema_migrations. Each migration
// runs in its own transaction; a partial failure rolls back that one
// migration but keeps prior ones. Subsequent runs pick up where we left
// off.
//
// Idempotent: running this on an up-to-date DB is a no-op (one SELECT).
func runMigrations(db *sql.DB) error {
	if _, err := db.Exec(migrationsTable); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	// Find the highest applied version. NULL coalesces to 0 so first-run
	// applies everything.
	var current int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if err := applyMigration(db, m); err != nil {
			return fmt.Errorf("apply migration %d (%s): %w", m.version, m.name, err)
		}
	}
	return nil
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
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		m.version, m.name, Now(),
	); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}
	return tx.Commit()
}
