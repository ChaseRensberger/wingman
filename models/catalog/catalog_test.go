package catalog

import "testing"

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
