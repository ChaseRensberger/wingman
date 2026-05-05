package storage_test

import (
	"path/filepath"
	"testing"

	"github.com/chaserensberger/wingman/storage"
)

func newStore(t *testing.T) storage.Store {
	t.Helper()
	_, store := newStoreWithPath(t)
	return store
}

func newStoreWithPath(t *testing.T) (string, storage.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return dbPath, store
}
