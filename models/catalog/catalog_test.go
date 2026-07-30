package catalog

import "testing"

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
