package store_test

import (
	"path/filepath"
	"testing"

	storepkg "github.com/chaserensberger/wingman/store"
)

func newStore(t *testing.T) storepkg.Store {
	t.Helper()
	_, store := newStoreWithPath(t)
	return store
}

func newStoreWithPath(t *testing.T) (string, storepkg.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := storepkg.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return dbPath, store
}
