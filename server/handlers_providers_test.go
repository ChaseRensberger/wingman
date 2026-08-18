package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chaserensberger/wingman/execution"
	provider "github.com/chaserensberger/wingman/models/providers"
)

func TestListProviderModelsReturnsEmptyMapForValidProviderWithoutModels(t *testing.T) {
	t.Parallel()

	registry, err := provider.NewRegistry(map[string]provider.ProviderConfig{
		"openai-compatible": {Name: "OpenAI Compatible"},
	})
	if err != nil {
		t.Fatal(err)
	}
	scopes, err := execution.NewManager(execution.Config{Providers: registry, DisablePlugins: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = scopes.Close() })
	server := New(Config{Scopes: scopes})
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/provider/openai-compatible/models", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var models map[string]ModelDTO
	if err := json.NewDecoder(response.Body).Decode(&models); err != nil {
		t.Fatal(err)
	}
	if len(models) != 0 {
		t.Fatalf("models = %#v, want empty map", models)
	}
}
