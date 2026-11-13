package pluginhost

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/chaserensberger/wingman/plugin/protocol"
)

func TestPluginStateIsolationAndPersistence(t *testing.T) {
	state := State{Root: t.TempDir()}
	ctx := WithIdentity(t.Context(), protocol.Identity{ClientID: "a", WorkDir: "/project"})
	if _, err := state.Storage(ctx, "acme/notes", "storage.set", json.RawMessage(`{"key":"note","value":"secret"}`)); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []struct{ plugin, client, directory string }{{"other/notes", "a", "/project"}, {"acme/notes", "b", "/project"}, {"acme/notes", "a", "/other"}} {
		value, err := state.Storage(WithIdentity(t.Context(), protocol.Identity{ClientID: scope.client, WorkDir: scope.directory}), scope.plugin, "storage.get", json.RawMessage(`{"key":"note"}`))
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(value)
		if string(data) != "null" {
			t.Fatalf("storage leaked across scope: %s", data)
		}
	}
	reopened := State{Root: state.Root}
	value, err := reopened.Storage(ctx, "acme/notes", "storage.get", json.RawMessage(`{"key":"note"}`))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(value)
	if string(data) != `"secret"` {
		t.Fatalf("storage did not survive reopen: %s", data)
	}
}

func TestProjectTrustRequiresExplicitCanonicalDecision(t *testing.T) {
	state := State{Root: t.TempDir()}
	dir := t.TempDir()
	trusted, err := state.Trusted(dir)
	if err != nil || trusted {
		t.Fatalf("unapproved directory = %v, %v", trusted, err)
	}
	if err := state.Trust(context.Background(), dir, true); err != nil {
		t.Fatal(err)
	}
	trusted, err = state.Trusted(filepath.Join(dir, "."))
	if err != nil || !trusted {
		t.Fatalf("canonical approval = %v, %v", trusted, err)
	}
	if err := state.Trust(context.Background(), dir, false); err != nil {
		t.Fatal(err)
	}
	trusted, err = state.Trusted(dir)
	if err != nil || trusted {
		t.Fatalf("revoked trust = %v, %v", trusted, err)
	}
}

func TestCredentialsAreClientScopedAndSeparateFromStorage(t *testing.T) {
	state := State{Root: t.TempDir()}
	ctx := WithIdentity(t.Context(), protocol.Identity{ClientID: "client", WorkDir: "/one"})
	input := json.RawMessage(`{"key":"native","value":{"type":"api_key","key":"secret"}}`)
	if _, err := state.Credential(ctx, "acme/native", "credential.set", input); err != nil {
		t.Fatal(err)
	}
	ctx = WithIdentity(t.Context(), protocol.Identity{ClientID: "client", WorkDir: "/two"})
	result, err := state.Credential(ctx, "acme/native", "credential.get", json.RawMessage(`{"key":"native"}`))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(result)
	if string(data) != `{"type":"api_key","key":"secret"}` {
		t.Fatalf("credential not shared across projects: %s", data)
	}
	for _, scope := range []struct{ plugin, client string }{{"acme/native:credentials", "client"}, {"acme/native", "other"}} {
		ctx := WithIdentity(t.Context(), protocol.Identity{ClientID: scope.client})
		result, err := state.Storage(ctx, scope.plugin, "storage.get", json.RawMessage(`{"key":"native"}`))
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(result)
		if string(data) != "null" {
			t.Fatalf("credential leaked into storage: %s", data)
		}
		result, err = state.Credential(ctx, scope.plugin, "credential.get", json.RawMessage(`{"key":"native"}`))
		if err != nil {
			t.Fatal(err)
		}
		data, _ = json.Marshal(result)
		if string(data) != "null" {
			t.Fatalf("credential leaked across owner: %s", data)
		}
	}
}
