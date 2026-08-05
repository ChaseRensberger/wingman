package store_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestAuthSessionLifecycleParity(t *testing.T) {
	for _, test := range []struct {
		name string
		open func(*testing.T) store.Store
	}{
		{"sqlite", func(t *testing.T) store.Store {
			data, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "wingman.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = data.Close() })
			return data
		}},
		{"memory", func(*testing.T) store.Store { return memory.NewStore() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := test.open(t)
			client, err := data.CreateClient("Test Client")
			if err != nil {
				t.Fatal(err)
			}
			active := &store.AuthSession{ClientID: client.ID, Owner: true, TokenHash: "active-hash"}
			if err := data.CreateAuthSession(active); err != nil {
				t.Fatal(err)
			}
			if active.ID == "" || active.CreatedAt == "" {
				t.Fatalf("created session = %#v", active)
			}

			authenticated, err := data.AuthenticateAuthSession("active-hash")
			if err != nil || authenticated == nil || authenticated.ID != active.ID || authenticated.ClientID != client.ID || !authenticated.Owner || authenticated.TokenHash != "" {
				t.Fatalf("authenticate = %#v, %v", authenticated, err)
			}
			listed, err := data.ListAuthSessions(client.ID)
			if err != nil || len(listed) != 1 || listed[0].ID != active.ID || listed[0].TokenHash != "" {
				t.Fatalf("list = %#v, %v", listed, err)
			}

			if err := data.RevokeAuthSession(active.ID); err != nil {
				t.Fatal(err)
			}
			if err := data.RevokeAuthSession(active.ID); err != nil {
				t.Fatal(err)
			}
			if authenticated, err := data.AuthenticateAuthSession("active-hash"); err != nil || authenticated != nil {
				t.Fatalf("authenticate revoked = %#v, %v", authenticated, err)
			}

			expired := &store.AuthSession{
				ClientID:  client.ID,
				TokenHash: "expired-hash",
				ExpiresAt: time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
			}
			if err := data.CreateAuthSession(expired); err != nil {
				t.Fatal(err)
			}
			if authenticated, err := data.AuthenticateAuthSession("expired-hash"); err != nil || authenticated != nil {
				t.Fatalf("authenticate expired = %#v, %v", authenticated, err)
			}

			if err := data.CreateAuthSession(&store.AuthSession{ClientID: "cli_missing", TokenHash: "missing-client-hash"}); err == nil {
				t.Fatal("CreateAuthSession succeeded for a missing client")
			}
		})
	}
}

func TestSQLiteAuthSessionsSurviveReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wingman.db")
	first, err := store.NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	client, err := first.CreateClient("Reopen Client")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.CreateAuthSession(&store.AuthSession{ClientID: client.ID, TokenHash: "reopen-hash"}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := store.NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	authenticated, err := second.AuthenticateAuthSession("reopen-hash")
	if err != nil || authenticated == nil || authenticated.ClientID != client.ID || authenticated.TokenHash != "" {
		t.Fatalf("authenticate after reopen = %#v, %v", authenticated, err)
	}
}
