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
