package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chaserensberger/wingman/models"
)

func TestClientPrepareUsesExplicitRouteForCustomModel(t *testing.T) {
	client := NewClient(nil)
	prepared, err := client.Prepare(context.Background(), models.Request{
		Model: models.ModelRef{
			Provider: "openai",
			ID:       "custom-model",
			API:      models.APIOpenAIResponses,
			BaseURL:  "https://example.test/v1",
		},
		Messages: []models.Message{models.NewUserText("hello")},
	})
	if err != nil {
		t.Fatalf("prepare custom route: %v", err)
	}
	if prepared.URL != "https://example.test/v1/responses" {
		t.Fatalf("url = %q, want custom base URL", prepared.URL)
	}
	if prepared.Body["model"] != "custom-model" {
		t.Fatalf("body model = %v, want custom-model", prepared.Body["model"])
	}
}

func TestClientPrepareRequiresRouteForUnknownModel(t *testing.T) {
	client := NewClient(nil)
	_, err := client.Prepare(context.Background(), models.Request{
		Model:    models.ModelRef{Provider: "openai", ID: "custom-model"},
		Messages: []models.Message{models.NewUserText("hello")},
	})
	if err == nil {
		t.Fatal("prepare unknown model succeeded without api/base_url")
	}
}

func TestClientPrepareAppliesProviderBaseURLOverride(t *testing.T) {
	client := NewClientWithConfig(nil, map[string]ProviderConfig{
		"openai": {Options: ProviderOptions{BaseURL: "https://gateway.test/v1"}},
	})
	prepared, err := client.Prepare(context.Background(), models.Request{
		Model:    models.ModelRef{Provider: "openai", ID: "gpt-5.5"},
		Messages: []models.Message{models.NewUserText("hello")},
	})
	if err != nil {
		t.Fatalf("prepare catalog route: %v", err)
	}
	if prepared.URL != "https://gateway.test/v1/responses" {
		t.Fatalf("url = %q, want provider base URL override", prepared.URL)
	}
}

func TestRegisterConfigAddsCustomProviderModel(t *testing.T) {
	RegisterConfig(map[string]ProviderConfig{
		"test-gateway": {
			Name:    "Test Gateway",
			Options: ProviderOptions{BaseURL: "https://gateway.test/v1"},
			Models: map[string]models.ModelInfo{
				"gpt-test": {
					API:          models.APIOpenAIResponses,
					Capabilities: models.ModelCapabilities{Tools: true},
				},
			},
		},
	})

	client := NewClientWithConfig(nil, map[string]ProviderConfig{
		"test-gateway": {Options: ProviderOptions{BaseURL: "https://gateway.test/v1"}},
	})
	prepared, err := client.Prepare(context.Background(), models.Request{
		Model:    models.ModelRef{Provider: "test-gateway", ID: "gpt-test"},
		Messages: []models.Message{models.NewUserText("hello")},
	})
	if err != nil {
		t.Fatalf("prepare config model: %v", err)
	}
	if prepared.URL != "https://gateway.test/v1/responses" {
		t.Fatalf("url = %q, want config model URL", prepared.URL)
	}
	if prepared.Body["model"] != "gpt-test" {
		t.Fatalf("body model = %v, want gpt-test", prepared.Body["model"])
	}
}

func TestClientPrepareAppliesProviderQueryOverride(t *testing.T) {
	client := NewClientWithConfig(nil, map[string]ProviderConfig{
		"openai": {Options: ProviderOptions{BaseURL: "https://gateway.test/v1", Query: map[string]string{"api-version": "preview"}}},
	})
	prepared, err := client.Prepare(context.Background(), models.Request{
		Model:    models.ModelRef{Provider: "openai", ID: "gpt-5.5"},
		Messages: []models.Message{models.NewUserText("hello")},
	})
	if err != nil {
		t.Fatalf("prepare catalog route: %v", err)
	}
	if prepared.URL != "https://gateway.test/v1/responses?api-version=preview" {
		t.Fatalf("url = %q, want query override", prepared.URL)
	}
}

func TestClientStreamAppliesCustomAuthHeader(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("X-API-Key")
		w.Header().Set("content-type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n"))
	}))
	defer server.Close()

	client := NewClientWithConfig(map[string]string{"test-gateway": "secret"}, map[string]ProviderConfig{
		"test-gateway": {
			Options: ProviderOptions{BaseURL: server.URL, AuthHeader: "X-API-Key"},
			Models: map[string]models.ModelInfo{
				"gpt-test": {API: models.APIOpenAICompletions},
			},
		},
	})
	RegisterConfig(client.Providers)
	stream, err := client.Stream(context.Background(), models.Request{
		Model:    models.ModelRef{Provider: "test-gateway", ID: "gpt-test"},
		Messages: []models.Message{models.NewUserText("hello")},
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	for range stream.Iter() {
	}
	if _, err := stream.Final(); err != nil {
		t.Fatalf("final: %v", err)
	}
	if gotAuth != "secret" {
		t.Fatalf("auth = %q, want secret", gotAuth)
	}
}
