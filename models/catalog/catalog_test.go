package catalog

import (
	"testing"

	"github.com/chaserensberger/wingman/models"
)

func TestNewSnapshotIsolatedAndOverlaysBuiltins(t *testing.T) {
	overlays := map[string]ProviderOverlay{
		"openai": {BaseURL: "https://example.test/v1", Models: map[string]models.ModelInfo{
			"gpt-5.6-terra": {Provider: "openai", ID: "gpt-5.6-terra", API: models.APIOpenAIResponses},
		}},
	}
	snapshot, err := New(overlays)
	if err != nil {
		t.Fatal(err)
	}
	overlays["openai"].Models["gpt-5.6-terra"] = models.ModelInfo{Provider: "openai", ID: "gpt-5.6-terra", API: models.APIAnthropicMessages, BaseURL: "https://mutated.test"}
	model, ok := snapshot.Get("openai", "gpt-5.6-terra")
	if !ok || model.BaseURL != "https://example.test/v1" || model.API != models.APIOpenAIResponses {
		t.Fatalf("snapshot model = %#v", model)
	}
	builtin, ok := Get("openai", "gpt-5.6-terra")
	if !ok || builtin.BaseURL == "https://example.test/v1" {
		t.Fatalf("built-in model leaked overlay: %#v", builtin)
	}
}

func TestOpenCodeNemotron3UltraFree(t *testing.T) {
	model, ok := Get("opencode", "nemotron-3-ultra-free")
	if !ok {
		t.Fatal("Nemotron 3 Ultra Free route not found")
	}
	if model.ContextWindow != 1_000_000 {
		t.Errorf("ContextWindow = %d, want 1000000", model.ContextWindow)
	}
	if model.MaxOutput != 128_000 {
		t.Errorf("MaxOutput = %d, want 128000", model.MaxOutput)
	}
	if !model.Capabilities.Tools || !model.Capabilities.Reasoning {
		t.Errorf("Capabilities = %#v, want tools and reasoning", model.Capabilities)
	}

	for _, canonical := range ListCanonicalModels() {
		if canonical.ID == "nvidia/nemotron-3-ultra-free" {
			if canonical.Lab != "nvidia" || canonical.Name != "Nemotron 3 Ultra Free" {
				t.Errorf("canonical model = %#v", canonical)
			}
			return
		}
	}
	t.Fatal("canonical Nemotron 3 Ultra Free model not found")
}

func TestOpenAIVariants(t *testing.T) {
	model, ok := Get("openai", "gpt-5.6-terra")
	if !ok {
		t.Fatal("GPT-5.6 Terra route not found")
	}
	want := []string{"none", "low", "medium", "high", "xhigh", "max"}
	if len(model.Variants) != len(want) {
		t.Fatalf("variants = %#v", model.Variants)
	}
	for i, id := range want {
		variant := model.Variants[i]
		if variant.ID != id {
			t.Errorf("variant %d = %q, want %q", i, variant.ID, id)
		}
		reasoning, ok := variant.ProviderOptions["reasoning"].(map[string]any)
		if !ok || reasoning["effort"] != id {
			t.Errorf("variant %q options = %#v", id, variant.ProviderOptions)
		}
	}
}

func TestNewRejectsInvalidOverlayVariants(t *testing.T) {
	for _, variants := range [][]models.ModelVariant{
		{{ID: ""}},
		{{ID: "high"}, {ID: "high"}},
	} {
		_, err := New(map[string]ProviderOverlay{"example": {
			BaseURL: "https://example.test/v1",
			Models: map[string]models.ModelInfo{"chat": {
				Provider: "example", ID: "chat", API: models.APIOpenAICompatible, Variants: variants,
			}},
		}})
		if err == nil {
			t.Fatalf("New accepted variants %#v", variants)
		}
	}
}

func TestProviderBaseURLOverlayRewritesBuiltInRoutes(t *testing.T) {
	c, err := New(map[string]ProviderOverlay{"openai": {BaseURL: "https://gateway.test/v1"}})
	if err != nil {
		t.Fatal(err)
	}
	info, ok := c.Get("openai", "gpt-5.6-terra")
	if !ok || info.BaseURL != "https://gateway.test/v1" {
		t.Fatalf("overlaid model = %#v", info)
	}
	for _, route := range c.ListRoutes() {
		if route.Provider == "openai" && route.ID == "gpt-5.6-terra" && route.BaseModel == "" {
			t.Fatal("provider overlay removed canonical base model")
		}
	}
}
