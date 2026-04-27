package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/chaserensberger/wingman/wingmodels"
)

// buildWireReq is a test helper that calls buildRequest and returns the
// underlying wire struct (without Stream=true so JSON is deterministic).
func buildWireReq(t *testing.T, req wingmodels.Request) (request, builtRequest) {
	t.Helper()
	c := &Client{model: "claude-test", maxTokens: defaultMaxTokens}
	built := c.buildRequest(req)
	return built.wire, built
}

// ---- ToolChoice -------------------------------------------------------

func TestBuildRequest_ToolChoice_Auto(t *testing.T) {
	req := wingmodels.Request{
		Messages: []wingmodels.Message{wingmodels.NewUserText("hi")},
		Tools:    []wingmodels.ToolDef{{Name: "bash", Description: "run bash", InputSchema: map[string]any{"type": "object"}}},
		// Zero ToolChoice → auto → omit tool_choice from wire.
	}
	wire, _ := buildWireReq(t, req)
	if wire.ToolChoice != nil {
		t.Errorf("expected nil tool_choice for auto mode, got %+v", wire.ToolChoice)
	}
}

func TestBuildRequest_ToolChoice_Required(t *testing.T) {
	req := wingmodels.Request{
		Messages:   []wingmodels.Message{wingmodels.NewUserText("hi")},
		Tools:      []wingmodels.ToolDef{{Name: "bash", Description: "run bash", InputSchema: map[string]any{"type": "object"}}},
		ToolChoice: wingmodels.ToolChoice{Mode: wingmodels.ToolChoiceRequired},
	}
	wire, _ := buildWireReq(t, req)
	if wire.ToolChoice == nil {
		t.Fatal("expected tool_choice to be set for required mode")
	}
	// Anthropic uses "any" for "required".
	if wire.ToolChoice.Type != "any" {
		t.Errorf("ToolChoice.Type = %q, want any", wire.ToolChoice.Type)
	}
}

func TestBuildRequest_ToolChoice_None(t *testing.T) {
	req := wingmodels.Request{
		Messages:   []wingmodels.Message{wingmodels.NewUserText("hi")},
		Tools:      []wingmodels.ToolDef{{Name: "bash", Description: "run bash", InputSchema: map[string]any{"type": "object"}}},
		ToolChoice: wingmodels.ToolChoice{Mode: wingmodels.ToolChoiceNone},
	}
	wire, _ := buildWireReq(t, req)
	if wire.ToolChoice == nil {
		t.Fatal("expected tool_choice to be set for none mode")
	}
	if wire.ToolChoice.Type != "none" {
		t.Errorf("ToolChoice.Type = %q, want none", wire.ToolChoice.Type)
	}
}

func TestBuildRequest_ToolChoice_SpecificTool(t *testing.T) {
	req := wingmodels.Request{
		Messages:   []wingmodels.Message{wingmodels.NewUserText("hi")},
		Tools:      []wingmodels.ToolDef{{Name: "bash", Description: "run bash", InputSchema: map[string]any{"type": "object"}}},
		ToolChoice: wingmodels.ToolChoice{Mode: wingmodels.ToolChoiceTool, Tool: "bash"},
	}
	wire, _ := buildWireReq(t, req)
	if wire.ToolChoice == nil {
		t.Fatal("expected tool_choice to be set for tool mode")
	}
	if wire.ToolChoice.Type != "tool" {
		t.Errorf("ToolChoice.Type = %q, want tool", wire.ToolChoice.Type)
	}
	if wire.ToolChoice.Name != "bash" {
		t.Errorf("ToolChoice.Name = %q, want bash", wire.ToolChoice.Name)
	}
}

func TestBuildRequest_ToolChoice_OmittedWhenNoTools(t *testing.T) {
	// ToolChoice should be ignored if no tools are present.
	req := wingmodels.Request{
		Messages:   []wingmodels.Message{wingmodels.NewUserText("hi")},
		ToolChoice: wingmodels.ToolChoice{Mode: wingmodels.ToolChoiceRequired},
		// no Tools
	}
	wire, _ := buildWireReq(t, req)
	if wire.ToolChoice != nil {
		t.Errorf("expected nil tool_choice when no tools are defined, got %+v", wire.ToolChoice)
	}
}

// ---- Thinking ---------------------------------------------------------

func TestBuildRequest_Thinking_BudgetBased(t *testing.T) {
	// Non-adaptive model (claude-3.x name) should use budget-based thinking.
	c := &Client{model: "claude-3-7-sonnet-20250219", maxTokens: 4096}
	req := wingmodels.Request{
		Messages: []wingmodels.Message{wingmodels.NewUserText("think")},
		Capabilities: wingmodels.Capabilities{
			Thinking: &wingmodels.ThinkingConfig{BudgetTokens: 2048},
		},
	}
	built := c.buildRequest(req)

	if built.wire.Thinking == nil {
		t.Fatal("expected thinking config in wire request")
	}
	if built.wire.Thinking.Type != "enabled" {
		t.Errorf("Thinking.Type = %q, want enabled", built.wire.Thinking.Type)
	}
	if built.wire.Thinking.BudgetTokens != 2048 {
		t.Errorf("Thinking.BudgetTokens = %d, want 2048", built.wire.Thinking.BudgetTokens)
	}
	if !built.needsThinkingBeta {
		t.Error("expected needsThinkingBeta = true for budget-based model")
	}
	// MaxTokens must exceed budget.
	if built.wire.MaxTokens <= 2048 {
		t.Errorf("MaxTokens = %d, must exceed budget 2048", built.wire.MaxTokens)
	}
}

func TestBuildRequest_Thinking_BudgetBased_DefaultBudget(t *testing.T) {
	// BudgetTokens == 0 should default to 1024.
	c := &Client{model: "claude-3-5-sonnet-20241022", maxTokens: 4096}
	req := wingmodels.Request{
		Messages:     []wingmodels.Message{wingmodels.NewUserText("think")},
		Capabilities: wingmodels.Capabilities{Thinking: &wingmodels.ThinkingConfig{}},
	}
	built := c.buildRequest(req)
	if built.wire.Thinking == nil {
		t.Fatal("expected thinking config")
	}
	if built.wire.Thinking.BudgetTokens != 1024 {
		t.Errorf("BudgetTokens = %d, want 1024 (default)", built.wire.Thinking.BudgetTokens)
	}
}

func TestBuildRequest_Thinking_Adaptive(t *testing.T) {
	// claude-opus-4 is an adaptive model.
	c := &Client{model: "claude-opus-4-5", maxTokens: 4096}
	req := wingmodels.Request{
		Messages:     []wingmodels.Message{wingmodels.NewUserText("think")},
		Capabilities: wingmodels.Capabilities{Thinking: &wingmodels.ThinkingConfig{Effort: "high"}},
	}
	built := c.buildRequest(req)

	if built.wire.Thinking == nil {
		t.Fatal("expected thinking config in wire request")
	}
	if built.wire.Thinking.Type != "adaptive" {
		t.Errorf("Thinking.Type = %q, want adaptive", built.wire.Thinking.Type)
	}
	if built.needsThinkingBeta {
		t.Error("adaptive models must NOT set the interleaved-thinking beta header")
	}
}

func TestBuildRequest_Thinking_Nil(t *testing.T) {
	// No thinking config → no thinking block in wire request.
	req := wingmodels.Request{
		Messages: []wingmodels.Message{wingmodels.NewUserText("hello")},
	}
	wire, _ := buildWireReq(t, req)
	if wire.Thinking != nil {
		t.Errorf("expected nil thinking, got %+v", wire.Thinking)
	}
}

// ---- JSON serialization sanity ----------------------------------------

func TestBuildRequest_ToolChoice_JSON(t *testing.T) {
	req := wingmodels.Request{
		Messages:   []wingmodels.Message{wingmodels.NewUserText("hi")},
		Tools:      []wingmodels.ToolDef{{Name: "edit", Description: "edit file", InputSchema: map[string]any{"type": "object"}}},
		ToolChoice: wingmodels.ToolChoice{Mode: wingmodels.ToolChoiceRequired},
	}
	wire, _ := buildWireReq(t, req)
	wire.Stream = false

	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	tc, ok := m["tool_choice"].(map[string]any)
	if !ok {
		t.Fatalf("tool_choice missing or wrong type in JSON: %s", data)
	}
	if tc["type"] != "any" {
		t.Errorf("tool_choice.type = %v, want any", tc["type"])
	}
}
