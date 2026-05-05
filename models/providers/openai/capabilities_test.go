package openai

import (
	"encoding/json"
	"testing"

	"github.com/chaserensberger/wingman/models"
)

func buildReq(t *testing.T, req models.Request) responsesRequest {
	t.Helper()
	c := &Client{model: "gpt-test", maxTokens: defaultMaxTokens, baseURL: defaultBaseURL}
	caps := models.ModelCapabilities{}
	return c.buildRequest(req, caps)
}

func buildReqWithCaps(t *testing.T, req models.Request, caps models.ModelCapabilities) responsesRequest {
	t.Helper()
	c := &Client{model: "o3-test", maxTokens: defaultMaxTokens, baseURL: defaultBaseURL}
	return c.buildRequest(req, caps)
}

// ---- ToolChoice -----------------------------------------------------------

func TestBuildRequest_ToolChoice_Auto(t *testing.T) {
	req := models.Request{
		Messages: []models.Message{models.NewUserText("hi")},
		Tools:    []models.ToolDef{{Name: "bash", Description: "run", InputSchema: map[string]any{"type": "object"}}},
	}
	r := buildReq(t, req)
	if r.ToolChoice != nil {
		t.Errorf("expected nil tool_choice for auto, got %v", r.ToolChoice)
	}
}

func TestBuildRequest_ToolChoice_Required(t *testing.T) {
	req := models.Request{
		Messages:   []models.Message{models.NewUserText("hi")},
		Tools:      []models.ToolDef{{Name: "bash", Description: "run", InputSchema: map[string]any{"type": "object"}}},
		ToolChoice: models.ToolChoice{Mode: models.ToolChoiceRequired},
	}
	r := buildReq(t, req)
	if r.ToolChoice != "required" {
		t.Errorf("ToolChoice = %v, want required", r.ToolChoice)
	}
}

func TestBuildRequest_ToolChoice_None(t *testing.T) {
	req := models.Request{
		Messages:   []models.Message{models.NewUserText("hi")},
		Tools:      []models.ToolDef{{Name: "bash", Description: "run", InputSchema: map[string]any{"type": "object"}}},
		ToolChoice: models.ToolChoice{Mode: models.ToolChoiceNone},
	}
	r := buildReq(t, req)
	if r.ToolChoice != "none" {
		t.Errorf("ToolChoice = %v, want none", r.ToolChoice)
	}
}

func TestBuildRequest_ToolChoice_SpecificTool(t *testing.T) {
	req := models.Request{
		Messages:   []models.Message{models.NewUserText("hi")},
		Tools:      []models.ToolDef{{Name: "bash", Description: "run", InputSchema: map[string]any{"type": "object"}}},
		ToolChoice: models.ToolChoice{Mode: models.ToolChoiceTool, Tool: "bash"},
	}
	r := buildReq(t, req)
	m, ok := r.ToolChoice.(map[string]any)
	if !ok {
		t.Fatalf("ToolChoice = %T, want map[string]any", r.ToolChoice)
	}
	if m["type"] != "function" {
		t.Errorf("ToolChoice.type = %v, want function", m["type"])
	}
	if m["name"] != "bash" {
		t.Errorf("ToolChoice.name = %v, want bash", m["name"])
	}
}

func TestBuildRequest_ToolChoice_OmittedWhenNoTools(t *testing.T) {
	req := models.Request{
		Messages:   []models.Message{models.NewUserText("hi")},
		ToolChoice: models.ToolChoice{Mode: models.ToolChoiceRequired},
	}
	r := buildReq(t, req)
	if r.ToolChoice != nil {
		t.Errorf("expected nil tool_choice when no tools, got %v", r.ToolChoice)
	}
}

// ---- Reasoning ------------------------------------------------------------

func TestBuildRequest_Reasoning_WithEffort(t *testing.T) {
	caps := models.ModelCapabilities{Reasoning: true}
	req := models.Request{
		Messages: []models.Message{models.NewUserText("think")},
		Capabilities: models.Capabilities{
			Thinking: &models.ThinkingConfig{Effort: "high"},
		},
	}
	r := buildReqWithCaps(t, req, caps)
	if r.Reasoning == nil {
		t.Fatal("expected reasoning config")
	}
	if r.Reasoning.Effort != "high" {
		t.Errorf("Reasoning.Effort = %q, want high", r.Reasoning.Effort)
	}
	if r.Reasoning.Summary != "auto" {
		t.Errorf("Reasoning.Summary = %q, want auto", r.Reasoning.Summary)
	}
	if len(r.Include) == 0 || r.Include[0] != "reasoning.encrypted_content" {
		t.Errorf("Include = %v, want [reasoning.encrypted_content]", r.Include)
	}
}

func TestBuildRequest_Reasoning_DefaultEffort(t *testing.T) {
	caps := models.ModelCapabilities{Reasoning: true}
	req := models.Request{
		Messages:     []models.Message{models.NewUserText("think")},
		Capabilities: models.Capabilities{Thinking: &models.ThinkingConfig{}},
	}
	r := buildReqWithCaps(t, req, caps)
	if r.Reasoning == nil {
		t.Fatal("expected reasoning config")
	}
	if r.Reasoning.Effort != "medium" {
		t.Errorf("Reasoning.Effort = %q, want medium (default)", r.Reasoning.Effort)
	}
}

func TestBuildRequest_Reasoning_NilThinking_DisablesReasoning(t *testing.T) {
	// Reasoning model but caller didn't ask for it → effort:none
	caps := models.ModelCapabilities{Reasoning: true}
	req := models.Request{
		Messages: []models.Message{models.NewUserText("hello")},
		// Capabilities.Thinking is nil
	}
	r := buildReqWithCaps(t, req, caps)
	if r.Reasoning == nil {
		t.Fatal("expected reasoning block with effort:none")
	}
	if r.Reasoning.Effort != "none" {
		t.Errorf("Reasoning.Effort = %q, want none", r.Reasoning.Effort)
	}
}

func TestBuildRequest_Reasoning_NonReasoningModel(t *testing.T) {
	// Non-reasoning model: no reasoning block even if Thinking config is present.
	caps := models.ModelCapabilities{Reasoning: false}
	req := models.Request{
		Messages:     []models.Message{models.NewUserText("hi")},
		Capabilities: models.Capabilities{Thinking: &models.ThinkingConfig{Effort: "high"}},
	}
	r := buildReqWithCaps(t, req, caps)
	if r.Reasoning != nil {
		t.Errorf("expected nil reasoning for non-reasoning model, got %+v", r.Reasoning)
	}
}

// ---- JSON sanity ----------------------------------------------------------

func TestBuildRequest_Store_AlwaysFalse(t *testing.T) {
	req := models.Request{Messages: []models.Message{models.NewUserText("hi")}}
	r := buildReq(t, req)
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	if m["store"] != false {
		t.Errorf("store = %v, want false", m["store"])
	}
}
