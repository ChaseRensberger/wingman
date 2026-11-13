package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/plugin/protocol"
	"github.com/chaserensberger/wingman/pluginhost"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestPluginHostAPICannotChangeClientIdentity(t *testing.T) {
	data := memory.NewStore()
	owner, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	other, err := data.CreateClient("Other")
	if err != nil {
		t.Fatal(err)
	}
	if err := data.CreateSession(&store.Session{ID: "private", ClientID: other.ID}); err != nil {
		t.Fatal(err)
	}
	s := New(Config{Store: data, Username: "test", Password: "private-password", PluginState: pluginhost.State{Root: t.TempDir()}})
	defer s.Close(t.Context())
	ctx := pluginhost.WithIdentity(t.Context(), protocol.Identity{ClientID: owner.ID})
	input, _ := json.Marshal(protocol.HTTPRequest{Method: http.MethodGet, Path: "/sessions/private?client_id=other"})
	result, err := s.pluginHostCall(ctx, "api.request", input)
	if err != nil {
		t.Fatal(err)
	}
	if response := result.(protocol.HTTPResponse); response.Status != http.StatusForbidden {
		t.Fatalf("cross-client access returned %d: %s", response.Status, response.Body)
	}
	body, _ := json.Marshal(api.CreateSessionRequest{Title: "Plugin child"})
	input, _ = json.Marshal(protocol.HTTPRequest{Method: http.MethodPost, Path: "/sessions", Body: body})
	result, err = s.pluginHostCall(ctx, "api.request", input)
	if err != nil {
		t.Fatal(err)
	}
	response := result.(protocol.HTTPResponse)
	if response.Status != http.StatusCreated {
		t.Fatalf("child creation returned %d: %s", response.Status, response.Body)
	}
	var child api.Session
	if err := json.Unmarshal(response.Body, &child); err != nil {
		t.Fatal(err)
	}
	saved, err := data.GetSession(child.ID)
	if err != nil || saved.ClientID != owner.ID {
		t.Fatalf("plugin created child in wrong client: %#v, %v", saved, err)
	}
	for _, path := range []string{"/clients", "/provider/auth", "/rpc-plugins/manage", "/rpc-plugins/trust", "https://example.com/sessions"} {
		input, _ := json.Marshal(protocol.HTTPRequest{Method: http.MethodGet, Path: path})
		if _, err := s.pluginHostCall(ctx, "api.request", input); err == nil {
			t.Fatalf("plugin could access %s", path)
		}
	}
	input, _ = json.Marshal(protocol.HTTPRequest{Method: "invalid\nmethod", Path: "/sessions"})
	if _, err := s.pluginHostCall(ctx, "api.request", input); err == nil {
		t.Fatal("invalid method was accepted")
	}
}
