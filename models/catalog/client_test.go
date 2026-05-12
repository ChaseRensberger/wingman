package catalog

import "testing"

func TestBundledSnapshotIncludesAnthropicDefault(t *testing.T) {
	info, ok := Get("anthropic", "claude-haiku-4.5")
	if !ok {
		t.Fatal("expected anthropic claude-haiku-4.5 in bundled catalog")
	}
	if info.ContextWindow != 200000 {
		t.Fatalf("ContextWindow = %d, want 200000", info.ContextWindow)
	}
	if info.MaxOutput != 64000 {
		t.Fatalf("MaxOutput = %d, want 64000", info.MaxOutput)
	}
	if !info.Capabilities.Tools {
		t.Fatal("expected tool support")
	}
	if !info.Capabilities.Images {
		t.Fatal("expected image input support")
	}
	if !info.Capabilities.Reasoning {
		t.Fatal("expected reasoning support")
	}
	if info.InputCostPerMTok != 1 {
		t.Fatalf("InputCostPerMTok = %f, want 1", info.InputCostPerMTok)
	}
}

func TestBundledSnapshotIncludesOpenAIDefault(t *testing.T) {
	info, ok := Get("openai", "gpt-5.5")
	if !ok {
		t.Fatal("expected openai gpt-5.5 in bundled catalog")
	}
	if !info.Capabilities.StructuredOutput {
		t.Fatal("expected structured output support")
	}
	if info.OutputCostPerMTok != 30 {
		t.Fatalf("OutputCostPerMTok = %f, want 30", info.OutputCostPerMTok)
	}
}

func TestBundledSnapshotIncludesOpenCodeZenDefault(t *testing.T) {
	models, ok := GetModels("opencode-zen")
	if !ok {
		t.Fatal("expected opencode-zen provider in bundled catalog")
	}
	if _, ok := models["claude-sonnet-4.6"]; !ok {
		t.Fatal("expected opencode-zen claude-sonnet-4.6 in bundled catalog")
	}
}

func TestCatalogCompilesFromTOMLData(t *testing.T) {
	snapshot, err := CompileDir("data")
	if err != nil {
		t.Fatalf("CompileDir returned error: %v", err)
	}
	if len(snapshot.ProviderModels) == 0 {
		t.Fatal("expected provider models from TOML data")
	}
}
