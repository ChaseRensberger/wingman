package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	storepkg "github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/storetest"
)

// TestMigrationsApplyCleanlyAndAreIdempotent asks: does opening a fresh DB
// apply all migrations, and does opening it again do nothing?
func TestMigrationsApplyCleanlyAndAreIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	store, err := storepkg.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("first open failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open direct connection: %v", err)
	}

	rows, err := db.Query(`SELECT version, name FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	defer rows.Close()

	var migrations []struct {
		version int
		name    string
	}
	for rows.Next() {
		var v int
		var n string
		if err := rows.Scan(&v, &n); err != nil {
			t.Fatalf("scan migration row: %v", err)
		}
		migrations = append(migrations, struct {
			version int
			name    string
		}{v, n})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}

	if len(migrations) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(migrations))
	}
	if migrations[0].version != 1 || migrations[0].name != "init" {
		t.Errorf("expected migration 1 'init', got %d %q", migrations[0].version, migrations[0].name)
	}

	firstCount := len(migrations)

	if err := db.Close(); err != nil {
		t.Fatalf("close direct connection: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	store, err = storepkg.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("second open failed: %v", err)
	}
	defer store.Close()

	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open direct connection second time: %v", err)
	}
	defer db.Close()

	var secondCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&secondCount); err != nil {
		t.Fatalf("count schema_migrations second time: %v", err)
	}
	if secondCount != firstCount {
		t.Errorf("expected %d migrations after re-open, got %d", firstCount, secondCount)
	}
}

// TestSQLiteConformance runs the shared conformance suite against a fresh
// on-disk SQLite store.
func TestSQLiteConformance(t *testing.T) {
	storetest.Run(t, func(t *testing.T) storepkg.Store {
		dbPath := filepath.Join(t.TempDir(), "test.db")
		store, err := storepkg.NewSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("failed to create test store: %v", err)
		}
		t.Cleanup(func() { store.Close() })
		return store
	})
}

// ---- SQLite-only raw SQL checks ------------------------------------------

func TestUpsertMessageUpdatesPreservesCreatedAtAndIdx_SQL(t *testing.T) {
	dbPath, store := newStoreWithPath(t)

	sess := &storepkg.Session{Title: "upsert-update", WorkDir: "/tmp"}
	if err := store.CreateSession(sess); err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	msgID := storepkg.NewID(storepkg.PrefixMessage)
	createdAt := time.Now().UTC().Add(-time.Hour)
	msg := storepkg.StoredMessage{
		ID:        msgID,
		SessionID: sess.ID,
		Idx:       5,
		Role:      "user",
		CreatedAt: createdAt,
	}
	if err := store.UpsertMessage(context.Background(), msg); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}

	msg.Role = "assistant"
	if err := store.UpsertMessage(context.Background(), msg); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var dbCreatedAt string
	if err := db.QueryRow("SELECT created_at FROM messages WHERE id = ?", msgID).Scan(&dbCreatedAt); err != nil {
		t.Fatalf("query created_at: %v", err)
	}
	parsed, err := time.Parse(time.RFC3339, dbCreatedAt)
	if err != nil {
		t.Fatalf("parse created_at: %v", err)
	}
	if !parsed.Truncate(time.Second).Equal(createdAt.Truncate(time.Second)) {
		t.Errorf("expected created_at preserved as %v, got %v", createdAt, parsed)
	}
}

func TestDeleteSessionCascadesToMessagesAndParts_SQL(t *testing.T) {
	dbPath, store := newStoreWithPath(t)

	sess := &storepkg.Session{Title: "cascade-test", WorkDir: "/tmp"}
	if err := store.CreateSession(sess); err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	msg := storepkg.StoredMessage{
		ID:        storepkg.NewID(storepkg.PrefixMessage),
		SessionID: sess.ID,
		Idx:       0,
		Role:      "user",
		CreatedAt: time.Now().UTC(),
	}
	if err := store.UpsertMessage(context.Background(), msg); err != nil {
		t.Fatalf("upsert message failed: %v", err)
	}
	part := storepkg.StoredPart{
		ID:          storepkg.NewID(storepkg.PrefixPart),
		MessageID:   msg.ID,
		Sequence:    0,
		Kind:        "text",
		PayloadJSON: []byte(`{"text":"hello"}`),
	}
	if err := store.UpsertPart(context.Background(), part); err != nil {
		t.Fatalf("upsert part failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open direct connection: %v", err)
	}
	defer db.Close()

	var msgCountBefore int
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, sess.ID).Scan(&msgCountBefore); err != nil {
		t.Fatalf("count messages before delete: %v", err)
	}
	if msgCountBefore == 0 {
		t.Fatal("expected messages before delete")
	}

	var partCountBefore int
	if err := db.QueryRow(`SELECT COUNT(*) FROM parts WHERE message_id IN (SELECT id FROM messages WHERE session_id = ?)`, sess.ID).Scan(&partCountBefore); err != nil {
		t.Fatalf("count parts before delete: %v", err)
	}
	if partCountBefore == 0 {
		t.Fatal("expected parts before delete")
	}

	if err := store.DeleteSession(sess.ID); err != nil {
		t.Fatalf("delete session failed: %v", err)
	}

	var msgCountAfter int
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, sess.ID).Scan(&msgCountAfter); err != nil {
		t.Fatalf("count messages after delete: %v", err)
	}
	if msgCountAfter != 0 {
		t.Errorf("expected 0 messages after delete, got %d", msgCountAfter)
	}

	var partCountAfter int
	if err := db.QueryRow(`SELECT COUNT(*) FROM parts WHERE message_id IN (SELECT id FROM messages WHERE session_id = ?)`, sess.ID).Scan(&partCountAfter); err != nil {
		t.Fatalf("count parts after delete: %v", err)
	}
	if partCountAfter != 0 {
		t.Errorf("expected 0 parts after delete, got %d", partCountAfter)
	}
}

func TestSessionWorkDirStoresNull_SQL(t *testing.T) {
	dbPath, store := newStoreWithPath(t)

	sess := &storepkg.Session{Title: "without-workdir"}
	if err := store.CreateSession(sess); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var workDir sql.NullString
	if err := db.QueryRow("SELECT work_dir FROM sessions WHERE id = ?", sess.ID).Scan(&workDir); err != nil {
		t.Fatalf("query work_dir: %v", err)
	}
	if workDir.Valid {
		t.Errorf("expected NULL work_dir in DB, got %q", workDir.String)
	}
}

func TestSessionCreatedWithNoClientIDIsNull_SQL(t *testing.T) {
	dbPath, store := newStoreWithPath(t)

	sess := &storepkg.Session{Title: "unscoped"}
	if err := store.CreateSession(sess); err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var clientID sql.NullString
	if err := db.QueryRow("SELECT client_id FROM sessions WHERE id = ?", sess.ID).Scan(&clientID); err != nil {
		t.Fatalf("query client_id: %v", err)
	}
	if clientID.Valid {
		t.Errorf("expected NULL client_id in DB, got %q", clientID.String)
	}
}

func TestUpdateSessionDoesNotChangeClientID_SQL(t *testing.T) {
	dbPath, store := newStoreWithPath(t)

	client, err := store.CreateClient("immutable-client")
	if err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	sess := &storepkg.Session{Title: "original", ClientID: client.ID}
	if err := store.CreateSession(sess); err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	sess.Title = "updated"
	if err := store.UpdateSession(sess); err != nil {
		t.Fatalf("update session failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var clientID sql.NullString
	if err := db.QueryRow("SELECT client_id FROM sessions WHERE id = ?", sess.ID).Scan(&clientID); err != nil {
		t.Fatalf("query client_id: %v", err)
	}
	if !clientID.Valid || clientID.String != client.ID {
		t.Errorf("expected client_id unchanged as %q, got %v", client.ID, clientID)
	}
}

func TestUpsertMessageWithNilMetadataJSONIsNull_SQL(t *testing.T) {
	dbPath, store := newStoreWithPath(t)

	sess := &storepkg.Session{Title: "nil-metadata", WorkDir: "/tmp"}
	if err := store.CreateSession(sess); err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	msg := storepkg.StoredMessage{
		ID:        storepkg.NewID(storepkg.PrefixMessage),
		SessionID: sess.ID,
		Idx:       0,
		Role:      "user",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.UpsertMessage(context.Background(), msg); err != nil {
		t.Fatalf("upsert message failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var isNull bool
	if err := db.QueryRow("SELECT metadata_json IS NULL FROM messages WHERE id = ?", msg.ID).Scan(&isNull); err != nil {
		t.Fatalf("query is null: %v", err)
	}
	if !isNull {
		t.Error("expected metadata_json to be SQL NULL")
	}
}

func TestUpsertMessageSecondCallUpdatesMetadataJSONAndUpdatedAt_SQL(t *testing.T) {
	dbPath, store := newStoreWithPath(t)

	sess := &storepkg.Session{Title: "update-metadata", WorkDir: "/tmp"}
	if err := store.CreateSession(sess); err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	createdAt := time.Now().UTC().Add(-time.Hour)
	msg := storepkg.StoredMessage{
		ID:           storepkg.NewID(storepkg.PrefixMessage),
		SessionID:    sess.ID,
		Idx:          7,
		Role:         "user",
		MetadataJSON: []byte(`{"v":1}`),
		CreatedAt:    createdAt,
		UpdatedAt:    createdAt,
	}
	if err := store.UpsertMessage(context.Background(), msg); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}

	msg.Role = "assistant"
	msg.MetadataJSON = []byte(`{"v":2}`)
	msg.UpdatedAt = time.Now().UTC()
	if err := store.UpsertMessage(context.Background(), msg); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var dbCreatedAt string
	if err := db.QueryRow("SELECT created_at FROM messages WHERE id = ?", msg.ID).Scan(&dbCreatedAt); err != nil {
		t.Fatalf("query created_at: %v", err)
	}
	parsed, err := time.Parse(time.RFC3339, dbCreatedAt)
	if err != nil {
		t.Fatalf("parse created_at: %v", err)
	}
	if !parsed.Truncate(time.Second).Equal(createdAt.Truncate(time.Second)) {
		t.Errorf("expected created_at preserved as %v, got %v", createdAt, parsed)
	}
}

func TestUpsertPartSecondCallUpdatesUpdatedAt_SQL(t *testing.T) {
	dbPath, store := newStoreWithPath(t)

	sess := &storepkg.Session{Title: "part-update-timestamps", WorkDir: "/tmp"}
	if err := store.CreateSession(sess); err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	msg := storepkg.StoredMessage{
		ID:        storepkg.NewID(storepkg.PrefixMessage),
		SessionID: sess.ID,
		Idx:       0,
		Role:      "user",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.UpsertMessage(context.Background(), msg); err != nil {
		t.Fatalf("upsert message failed: %v", err)
	}

	createdAt := time.Now().UTC().Add(-time.Hour)
	part := storepkg.StoredPart{
		ID:          storepkg.NewID(storepkg.PrefixPart),
		MessageID:   msg.ID,
		Sequence:    5,
		Kind:        "text",
		PayloadJSON: []byte(`{"text":"first"}`),
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}
	if err := store.UpsertPart(context.Background(), part); err != nil {
		t.Fatalf("first upsert part failed: %v", err)
	}

	part.Kind = "tool_call"
	part.PayloadJSON = []byte(`{"call_id":"c1"}`)
	part.UpdatedAt = time.Now().UTC()
	if err := store.UpsertPart(context.Background(), part); err != nil {
		t.Fatalf("second upsert part failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var dbUpdatedAt string
	if err := db.QueryRow("SELECT updated_at FROM parts WHERE id = ?", part.ID).Scan(&dbUpdatedAt); err != nil {
		t.Fatalf("query updated_at: %v", err)
	}
	parsed, err := time.Parse(time.RFC3339, dbUpdatedAt)
	if err != nil {
		t.Fatalf("parse updated_at: %v", err)
	}
	if !parsed.After(createdAt) {
		t.Errorf("expected updated_at to be after created_at, got %v", parsed)
	}
}
