package ollama

import (
	"encoding/json"
	"testing"

	"github.com/chaserensberger/wingman/wingmodels"
)

func buildOllamaWire(t *testing.T, req wingmodels.Request) request {
	t.Helper()
	c := &Client{model: "llama-test", maxTokens: 4096}
	return c.buildRequest(req)
}

// ---- ToolChoice -------------------------------------------------------

func TestOllama_BuildRequest_ToolChoice_Auto(t *testing.T) {
	req := wingmodels.Request{
		Messages: []wingmodels.Message{wingmodels.NewUserText("hi")},
		Tools:    []wingmodels.ToolDef{{Name: "bash", Description: "run", InputSchema: map[string]any{"type": "object"}}},
		// zero ToolChoice → omit field
	}
	wire := buildOllamaWire(t, req)
	if wire.ToolChoice != "" {
		t.Errorf("expected empty tool_choice for auto mode, got %q", wire.ToolChoice)
	}
}

func TestOllama_BuildRequest_ToolChoice_None(t *testing.T) {
	req := wingmodels.Request{
		Messages:   []wingmodels.Message{wingmodels.NewUserText("hi")},
		Tools:      []wingmodels.ToolDef{{Name: "bash", Description: "run", InputSchema: map[string]any{"type": "object"}}},
		ToolChoice: wingmodels.ToolChoice{Mode: wingmodels.ToolChoiceNone},
	}
	wire := buildOllamaWire(t, req)
	if wire.ToolChoice != "none" {
		t.Errorf("ToolChoice = %q, want none", wire.ToolChoice)
	}
}

func TestOllama_BuildRequest_ToolChoice_Required(t *testing.T) {
	req := wingmodels.Request{
		Messages:   []wingmodels.Message{wingmodels.NewUserText("hi")},
		Tools:      []wingmodels.ToolDef{{Name: "bash", Description: "run", InputSchema: map[string]any{"type": "object"}}},
		ToolChoice: wingmodels.ToolChoice{Mode: wingmodels.ToolChoiceRequired},
	}
	wire := buildOllamaWire(t, req)
	if wire.ToolChoice != "required" {
		t.Errorf("ToolChoice = %q, want required", wire.ToolChoice)
	}
}

func TestOllama_BuildRequest_ToolChoice_OmittedWithNoTools(t *testing.T) {
	req := wingmodels.Request{
		Messages:   []wingmodels.Message{wingmodels.NewUserText("hi")},
		ToolChoice: wingmodels.ToolChoice{Mode: wingmodels.ToolChoiceRequired},
		// no Tools — choice should be ignored
	}
	wire := buildOllamaWire(t, req)
	if wire.ToolChoice != "" {
		t.Errorf("expected empty tool_choice when no tools defined, got %q", wire.ToolChoice)
	}
}

// ---- JSON sanity -------------------------------------------------------

func TestOllama_BuildRequest_ToolChoice_JSON(t *testing.T) {
	req := wingmodels.Request{
		Messages:   []wingmodels.Message{wingmodels.NewUserText("hi")},
		Tools:      []wingmodels.ToolDef{{Name: "bash", Description: "run", InputSchema: map[string]any{"type": "object"}}},
		ToolChoice: wingmodels.ToolChoice{Mode: wingmodels.ToolChoiceNone},
	}
	wire := buildOllamaWire(t, req)

	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if m["tool_choice"] != "none" {
		t.Errorf("tool_choice in JSON = %v, want none", m["tool_choice"])
	}
}
