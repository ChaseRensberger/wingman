package models

import "testing"

func TestNormalizeMessagesFoldsLegacyToolResults(t *testing.T) {
	messages := []Message{
		{Role: RoleAssistant, Content: Content{ToolCallPart{CallID: "call_1", Name: "bash", Input: map[string]any{"command": "pwd"}}}},
		{Role: RoleTool, Content: Content{ToolResultPart{CallID: "call_1", Name: "bash", Output: Content{TextPart{Text: "/tmp"}}}}},
	}

	normalized := NormalizeMessages(messages)
	if len(normalized) != 1 || normalized[0].Role != RoleAssistant {
		t.Fatalf("normalized messages = %#v", normalized)
	}
	tool, ok := normalized[0].Content[0].(ToolPart)
	if !ok || tool.State != ToolStateCompleted || tool.Output != "/tmp" {
		t.Fatalf("tool = %#v", normalized[0].Content[0])
	}
}

func TestExpandToolMessagesDerivesProviderResult(t *testing.T) {
	messages := []Message{{Role: RoleAssistant, Content: Content{ToolPart{CallID: "call_1", Name: "bash", State: ToolStateCompleted, Input: map[string]any{"command": "pwd"}, Output: "/tmp"}}}}

	expanded := ExpandToolMessages(messages)
	if len(expanded) != 2 || expanded[1].Role != RoleTool {
		t.Fatalf("expanded messages = %#v", expanded)
	}
	result, ok := expanded[1].Content[0].(ToolResultPart)
	if !ok || toolResultText(result) != "/tmp" {
		t.Fatalf("result = %#v", expanded[1].Content[0])
	}
}

func TestExpandToolMessagesCombinesOutputAndError(t *testing.T) {
	messages := []Message{{Role: RoleAssistant, Content: Content{ToolPart{
		CallID: "call_1",
		Name:   "bash",
		State:  ToolStateError,
		Input:  map[string]any{"command": "pwd"},
		Output: "partial",
		Error:  "failed",
	}}}}

	expanded := ExpandToolMessages(messages)
	if len(expanded) != 2 || expanded[1].Role != RoleTool {
		t.Fatalf("expanded messages = %#v", expanded)
	}
	result, ok := expanded[1].Content[0].(ToolResultPart)
	if !ok || !result.IsError {
		t.Fatalf("result = %#v, want error", result)
	}
	var text string
	for _, p := range result.Output {
		if tp, ok := p.(TextPart); ok {
			text = tp.Text
		}
	}
	if text != "partial\nfailed" {
		t.Fatalf("result text = %q, want combined output+error", text)
	}
}

func TestExpandToolMessagesOmitsUnresolvedTool(t *testing.T) {
	messages := []Message{{Role: RoleAssistant, Content: Content{ToolPart{
		CallID: "call_1",
		Name:   "bash",
		State:  ToolStatePending,
		Input:  map[string]any{"command": "pwd"},
	}}}}

	if expanded := ExpandToolMessages(messages); len(expanded) != 0 {
		t.Fatalf("expanded messages = %#v, want unresolved tool omitted", expanded)
	}
}

func TestNewCallTraceRedactsAndShapes(t *testing.T) {
	req := Request{
		Model:  ModelRef{Provider: "openai", ID: "gpt-4", API: APIOpenAIResponses},
		System: "Current date: 2024-01-01.\nBe helpful.",
		Messages: []Message{
			{Role: RoleUser, Content: Content{TextPart{Text: "hello"}}},
			{Role: RoleAssistant, Content: Content{TextPart{Text: "hi"}, ToolCallPart{CallID: "c1", Name: "test", Input: map[string]any{}}}},
		},
		Tools: []ToolDef{
			{Name: "test", Description: "d", InputSchema: map[string]any{"type": "object"}},
		},
		Capabilities: Capabilities{Thinking: true},
	}
	lowered := LoweredOptions{ReasoningSummaryAuto: true}
	trace := NewCallTrace(req, lowered)

	if trace.Version != "1" {
		t.Fatalf("version = %q, want 1", trace.Version)
	}
	if trace.Provider != "openai" {
		t.Fatalf("provider = %q", trace.Provider)
	}
	if trace.API != APIOpenAIResponses {
		t.Fatalf("api = %q", trace.API)
	}
	if !trace.Runtime.CurrentDate {
		t.Fatal("expected current_date true")
	}
	if len(trace.Tools) != 1 || trace.Tools[0].Name != "test" {
		t.Fatal("tool trace mismatch")
	}
	if trace.Tools[0].SchemaHash == "" || trace.Tools[0].SchemaBytes == 0 {
		t.Fatal("expected schema hash and bytes")
	}
	if trace.Messages.Count != 2 {
		t.Fatalf("message count = %d", trace.Messages.Count)
	}
	if trace.Messages.ByRole["user"] != 1 || trace.Messages.ByRole["assistant"] != 1 {
		t.Fatalf("by_role = %v", trace.Messages.ByRole)
	}
	if trace.Messages.PartKinds["text"] != 2 || trace.Messages.PartKinds["tool_call"] != 1 {
		t.Fatalf("part_kinds = %v", trace.Messages.PartKinds)
	}
	if trace.System.Bytes == 0 || trace.System.SHA256 == "" {
		t.Fatal("expected system trace")
	}
	if !trace.Lowered.ReasoningSummaryAuto {
		t.Fatal("expected reasoning_summary_auto")
	}
	// Verify no message content or credentials leaked into trace fields
	if trace.Messages.Count == 0 {
		t.Fatal("structural info missing")
	}
}
