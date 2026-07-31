package provider_test

import (
	"context"
	"testing"

	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/models/providers/openai"
	"github.com/chaserensberger/wingman/models/providers/opencodego"
)

func TestOpenCodeGoKimiK3Route(t *testing.T) {
	client := provider.NewClient(map[string]string{opencodego.ID: "test-key"})
	prepared, err := client.Prepare(context.Background(), models.Request{
		Model:    opencodego.Model("kimi-k3"),
		Messages: []models.Message{models.NewUserText("hello")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.URL != "https://opencode.ai/zen/go/v1/chat/completions" {
		t.Errorf("URL = %q", prepared.URL)
	}
	if prepared.API != models.APIOpenAICompatible {
		t.Errorf("API = %q, want %q", prepared.API, models.APIOpenAICompatible)
	}
	if prepared.Body["model"] != "kimi-k3" {
		t.Errorf("model = %v, want kimi-k3", prepared.Body["model"])
	}
}

func TestOpenAIOAuthUsesCodexRoute(t *testing.T) {
	client := provider.NewClientWithCredentials(map[string]provider.Credential{
		openai.ID: {Type: "oauth", Access: "access", Refresh: "refresh", ExpiresAt: 1},
	}, nil)
	prepared, err := client.Prepare(context.Background(), models.Request{
		Model:    openai.Model("gpt-5.6-terra"),
		Messages: []models.Message{models.NewUserText("hello")},
		HTTP:     models.HTTPOptions{Body: map[string]any{"store": true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.URL != "https://chatgpt.com/backend-api/codex/responses" {
		t.Errorf("URL = %q", prepared.URL)
	}
	if prepared.Headers["originator"] != "codex_cli_rs" {
		t.Errorf("originator = %q", prepared.Headers["originator"])
	}
	if prepared.Body["store"] != false {
		t.Errorf("store = %#v, want false", prepared.Body["store"])
	}
}

func TestClientReportsOpenAIReasoningSummaryLowering(t *testing.T) {
	client := provider.NewClient(map[string]string{openai.ID: "test-key"})
	opts := client.LoweredOptions(context.Background(), models.Request{
		Model:        openai.Model("gpt-5.6-terra"),
		Capabilities: models.Capabilities{Thinking: true},
	})
	if !opts.ReasoningSummaryAuto {
		t.Fatal("ReasoningSummaryAuto = false, want true")
	}
}

func TestRegistrySnapshotsConfigWithoutLeakingToOtherGenerations(t *testing.T) {
	env := []string{"EXAMPLE_KEY"}
	configs := map[string]provider.ProviderConfig{
		"example": {
			Name:    "Example",
			Options: provider.ProviderOptions{BaseURL: "https://example.test/v1", Query: map[string]string{"version": "1"}},
			Models: map[string]models.ModelInfo{
				"chat": {Provider: "example", ID: "chat", API: models.APIOpenAICompatible, Env: env},
			},
		},
	}
	generation, err := provider.NewRegistry(configs)
	if err != nil {
		t.Fatal(err)
	}
	configs["example"].Models["chat"] = models.ModelInfo{Provider: "example", ID: "chat", API: models.APIAnthropicMessages, BaseURL: "https://mutated.test"}
	env[0] = "MUTATED_KEY"
	model, ok := generation.Catalog().Get("example", "chat")
	if !ok || model.BaseURL != "https://example.test/v1" || model.API != models.APIOpenAICompatible || model.Env[0] != "EXAMPLE_KEY" {
		t.Fatalf("generation model = %#v", model)
	}
	if !generation.IsValid("example") {
		t.Fatal("custom provider is absent from generation")
	}
	for i, meta := range generation.List() {
		if i > 0 && generation.List()[i-1].ID > meta.ID {
			t.Fatalf("provider order is not deterministic: %#v", generation.List())
		}
	}
	clean, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	if clean.IsValid("example") {
		t.Fatal("custom provider leaked to next generation")
	}
	if _, ok := clean.Catalog().Get("example", "chat"); ok {
		t.Fatal("custom model leaked to next generation")
	}
}

func TestRegistryInvalidConfigDoesNotMutateBuiltins(t *testing.T) {
	_, err := provider.NewRegistry(map[string]provider.ProviderConfig{
		"invalid": {Options: provider.ProviderOptions{BaseURL: "https://invalid.test"}, Models: map[string]models.ModelInfo{
			"chat": {Provider: "invalid", ID: "chat", API: "unsupported"},
		}},
	})
	if err == nil {
		t.Fatal("NewRegistry accepted unsupported model API")
	}
	if provider.IsValid("invalid") {
		t.Fatal("invalid provider mutated built-in registry")
	}
	clean, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	if clean.IsValid("invalid") {
		t.Fatal("invalid provider leaked into a later generation")
	}
}

func TestRegistryDefaultsModelIdentityAndAllowsMetadataOnlyProvider(t *testing.T) {
	registry, err := provider.NewRegistry(map[string]provider.ProviderConfig{
		"metadata-only": {Name: "Metadata only"},
		"example": {
			Options: provider.ProviderOptions{BaseURL: "https://example.test/v1"},
			Models:  map[string]models.ModelInfo{"chat": {API: models.APIOpenAICompatible}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !registry.IsValid("metadata-only") {
		t.Fatal("metadata-only provider is absent")
	}
	info, ok := registry.Catalog().Get("example", "chat")
	if !ok || info.Provider != "example" || info.ID != "chat" || info.BaseURL != "https://example.test/v1" {
		t.Fatalf("defaulted model = %#v", info)
	}
}

func TestRegistryProviderBaseURLOverridesBuiltInModelRoutes(t *testing.T) {
	registry, err := provider.NewRegistry(map[string]provider.ProviderConfig{
		openai.ID: {Options: provider.ProviderOptions{BaseURL: "https://gateway.test/v1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := registry.NewClient(map[string]string{openai.ID: "key"}).Prepare(context.Background(), models.Request{
		Model: openai.Model("gpt-5.6-terra"), Messages: []models.Message{models.NewUserText("hello")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.URL != "https://gateway.test/v1/responses" {
		t.Fatalf("prepared URL = %q", prepared.URL)
	}
}
