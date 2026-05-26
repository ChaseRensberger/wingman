package store

import (
	"errors"
	"testing"
)

func TestSQLiteStoreDefaultClientAndUniqueNames(t *testing.T) {
	st, err := NewSQLiteStore(t.TempDir() + "/wingman.db")
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	defer st.Close()

	client, err := st.EnsureDefaultClient()
	if err != nil {
		t.Fatalf("ensure default client: %v", err)
	}
	if client.ID != DefaultClientID || client.Name != DefaultClientName {
		t.Fatalf("unexpected default client: %#v", client)
	}

	if _, err := st.CreateClient("my-app"); err != nil {
		t.Fatalf("create client: %v", err)
	}
	if _, err := st.CreateClient("MY-APP"); !errors.Is(err, ErrClientNameExists) {
		t.Fatalf("expected ErrClientNameExists, got %v", err)
	}
	if _, err := st.CreateClient(DefaultClientName); !errors.Is(err, ErrClientNameExists) {
		t.Fatalf("expected ErrClientNameExists for default name, got %v", err)
	}
}
