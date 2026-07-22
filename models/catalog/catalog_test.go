package catalog

import "testing"

func TestOpenCodeGoCatalogIncludesKimiK3(t *testing.T) {
	info, ok := Get("opencode-go", "kimi-k3")
	if !ok {
		t.Fatal("OpenCode Go catalog missing kimi-k3")
	}
	if info.API != "openai_compatible_chat" {
		t.Errorf("Kimi K3 API = %q, want openai_compatible_chat", info.API)
	}
	if info.BaseURL != "https://opencode.ai/zen/go/v1" {
		t.Errorf("Kimi K3 base URL = %q", info.BaseURL)
	}
	if info.ContextWindow != 1048576 || info.MaxOutput != 131072 {
		t.Errorf("Kimi K3 limits = %d/%d, want 1048576/131072", info.ContextWindow, info.MaxOutput)
	}
	if !info.Capabilities.Tools || !info.Capabilities.Images || !info.Capabilities.Reasoning || !info.Capabilities.StructuredOutput {
		t.Errorf("Kimi K3 capabilities = %+v, want all supported capabilities", info.Capabilities)
	}
}

func TestOpenAICatalogUsesGPT56Models(t *testing.T) {
	for _, id := range []string{"gpt-5.6-luna", "gpt-5.6-terra", "gpt-5.6-sol"} {
		info, ok := Get("openai", id)
		if !ok {
			t.Errorf("OpenAI catalog missing %q", id)
			continue
		}
		if !info.Capabilities.StructuredOutput {
			t.Errorf("OpenAI model %q does not support structured output", id)
		}
	}

	for _, ref := range []string{
		"openai/gpt-5.5",
		"openai/gpt-5.5-pro",
		"opencode/gpt-5.5",
		"opencode/gpt-5.5-pro",
	} {
		if _, ok := GetRef(ref); ok {
			t.Errorf("retired model %q remains in the catalog", ref)
		}
	}
}
