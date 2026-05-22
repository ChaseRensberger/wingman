package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	provider "github.com/chaserensberger/wingman/models/providers"
	_ "github.com/chaserensberger/wingman/models/providers/openai"
)

func TestProviderEndpointReportsEffectiveRoute(t *testing.T) {
	t.Parallel()

	auth := false
	srv := New(Config{Providers: map[string]provider.ProviderConfig{
		"openai": {Options: provider.ProviderOptions{BaseURL: "https://gateway.test/v1", Auth: &auth}},
	}})

	req := httptest.NewRequest(http.MethodGet, "/provider/openai", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var got ProviderDTO
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Route.BaseURL != "https://gateway.test/v1" {
		t.Fatalf("base_url = %q, want configured gateway", got.Route.BaseURL)
	}
	if got.Route.BaseURLSource != "config" {
		t.Fatalf("base_url_source = %q, want config", got.Route.BaseURLSource)
	}
	if got.Route.AuthEnabled {
		t.Fatal("auth_enabled = true, want false")
	}
	if got.Route.AuthSource != "config" {
		t.Fatalf("auth_source = %q, want config", got.Route.AuthSource)
	}
	if got.Auth.Source != "disabled" {
		t.Fatalf("auth.source = %q, want disabled", got.Auth.Source)
	}
}
