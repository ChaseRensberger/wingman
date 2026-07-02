package httpmodel

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/models"
)

func TestAnthropicParsesStreamedToolCall(t *testing.T) {
	m := &Model{Info_: models.ModelInfo{Provider: "anthropic", ID: "claude", API: models.APIAnthropicMessages}, Protocol: AnthropicMessages}
	stream := models.NewEventStream[models.StreamPart, *models.Message](16)
	msg, usage, reason, err := m.readSSE(strings.NewReader(sseEvents(
		map[string]any{"type": "message_start", "message": map[string]any{"usage": map[string]any{"input_tokens": 5}}},
		map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "tool_use", "id": "call_1", "name": "lookup"}},
		map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": `{"query"`}},
		map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": `:"weather"}`}},
		map[string]any{"type": "content_block_stop", "index": 0},
		map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "tool_use"}, "usage": map[string]any{"output_tokens": 1}},
	)), stream)
	if err != nil {
		t.Fatalf("readSSE: %v", err)
	}
	if reason != models.FinishReasonToolCalls {
		t.Fatalf("reason = %q, want tool_calls", reason)
	}
	if usage.TotalTokens != 1 {
		t.Fatalf("usage total = %d, want 1", usage.TotalTokens)
	}
	if len(msg.Content) != 1 {
		t.Fatalf("content len = %d, want 1", len(msg.Content))
	}
	call, ok := msg.Content[0].(models.ToolCallPart)
	if !ok {
		t.Fatalf("content[0] = %T, want ToolCallPart", msg.Content[0])
	}
	if call.CallID != "call_1" || call.Name != "lookup" || call.Input["query"] != "weather" {
		t.Fatalf("call = %#v", call)
	}
}

func TestOpenAIResponsesParsesStreamedFunctionCallArguments(t *testing.T) {
	m := &Model{Info_: models.ModelInfo{Provider: "openai", ID: "gpt", API: models.APIOpenAIResponses}, Protocol: OpenAIResponses}
	stream := models.NewEventStream[models.StreamPart, *models.Message](16)
	msg, _, reason, err := m.readSSE(strings.NewReader(sseEvents(
		map[string]any{"type": "response.output_item.added", "item": map[string]any{"type": "function_call", "id": "item_1", "call_id": "call_1", "name": "lookup", "arguments": ""}},
		map[string]any{"type": "response.function_call_arguments.delta", "item_id": "item_1", "delta": `{"query"`},
		map[string]any{"type": "response.function_call_arguments.delta", "item_id": "item_1", "delta": `:"weather"}`},
		map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "function_call", "id": "item_1", "call_id": "call_1", "name": "lookup"}},
		map[string]any{"type": "response.completed", "response": map[string]any{"usage": map[string]any{"input_tokens": 5, "output_tokens": 1, "total_tokens": 6}}},
	)), stream)
	if err != nil {
		t.Fatalf("readSSE: %v", err)
	}
	if reason != models.FinishReasonToolCalls {
		t.Fatalf("reason = %q, want tool_calls", reason)
	}
	call, ok := msg.Content[0].(models.ToolCallPart)
	if !ok {
		t.Fatalf("content[0] = %T, want ToolCallPart", msg.Content[0])
	}
	if call.CallID != "call_1" || call.Name != "lookup" || call.Input["query"] != "weather" {
		t.Fatalf("call = %#v", call)
	}
}

func TestAnthropicPrepareIncludesToolHeadersWithoutAPIKey(t *testing.T) {
	m := &Model{Info_: models.ModelInfo{Provider: "anthropic", ID: "claude", API: models.APIAnthropicMessages}, Protocol: AnthropicMessages, BaseURL: "https://api.anthropic.test/v1"}
	prepared, err := m.Prepare(nil, models.Request{Messages: []models.Message{models.NewUserText("hi")}})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if prepared.Headers["anthropic-version"] == "" {
		t.Fatal("anthropic-version header missing")
	}
	if !strings.Contains(prepared.Headers["anthropic-beta"], "fine-grained-tool-streaming") {
		t.Fatalf("anthropic-beta = %q", prepared.Headers["anthropic-beta"])
	}
}

func TestOpenAIResponsesPrepareLowersImagesAndStructuredOutput(t *testing.T) {
	m := &Model{Info_: models.ModelInfo{Provider: "openai", ID: "gpt", API: models.APIOpenAIResponses}, Protocol: OpenAIResponses, BaseURL: "https://api.openai.test/v1"}
	prepared, err := m.Prepare(nil, models.Request{
		Messages: []models.Message{{Role: models.RoleUser, Content: models.Content{
			models.TextPart{Text: "describe"},
			models.ImagePart{Base64: "aGVsbG8=", MediaType: "image/png"},
		}}},
		OutputSchema: &models.OutputSchema{Name: "description", Schema: map[string]any{"type": "object"}, Strict: true},
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	input := prepared.Body["input"].([]any)
	content := input[0].(map[string]any)["content"].([]any)
	if content[1].(map[string]any)["type"] != "input_image" {
		t.Fatalf("image content = %#v", content[1])
	}
	text := prepared.Body["text"].(map[string]any)
	format := text["format"].(map[string]any)
	if format["type"] != "json_schema" || format["name"] != "description" || format["strict"] != true {
		t.Fatalf("format = %#v", format)
	}
}

func TestGeminiPrepareLowersMessagesToolsAndStructuredOutput(t *testing.T) {
	m := &Model{Info_: models.ModelInfo{Provider: "google", ID: "gemini-3.5-flash", API: models.APIGeminiGenerate}, Protocol: GeminiGenerate, BaseURL: "https://generativelanguage.googleapis.com/v1beta"}
	prepared, err := m.Prepare(nil, models.Request{
		System: "You are concise.",
		Messages: []models.Message{{Role: models.RoleUser, Content: models.Content{
			models.TextPart{Text: "describe"},
			models.ImagePart{Base64: "aGVsbG8=", MediaType: "image/png"},
		}}},
		Tools:        []models.ToolDef{{Name: "lookup", Description: "Look up data", InputSchema: map[string]any{"type": "object"}}},
		OutputSchema: &models.OutputSchema{Name: "description", Schema: map[string]any{"type": "object"}, Strict: true},
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if prepared.URL != "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.5-flash:streamGenerateContent?alt=sse" {
		t.Fatalf("url = %q", prepared.URL)
	}
	contents := prepared.Body["contents"].([]any)
	parts := contents[0].(map[string]any)["parts"].([]any)
	if parts[0].(map[string]any)["text"] != "describe" {
		t.Fatalf("text part = %#v", parts[0])
	}
	if parts[1].(map[string]any)["inlineData"] == nil {
		t.Fatalf("image part = %#v", parts[1])
	}
	tools := prepared.Body["tools"].([]any)
	decls := tools[0].(map[string]any)["functionDeclarations"].([]any)
	if decls[0].(map[string]any)["name"] != "lookup" {
		t.Fatalf("declarations = %#v", decls)
	}
	config := prepared.Body["generationConfig"].(map[string]any)
	if config["responseMimeType"] != "application/json" || config["responseSchema"] == nil {
		t.Fatalf("generationConfig = %#v", config)
	}
}

func TestGeminiParsesTextToolCallAndUsage(t *testing.T) {
	m := &Model{Info_: models.ModelInfo{Provider: "google", ID: "gemini-3.5-flash", API: models.APIGeminiGenerate}, Protocol: GeminiGenerate}
	stream := models.NewEventStream[models.StreamPart, *models.Message](16)
	textEvent := map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": "hi"}}}}}}
	toolEvent := map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{"parts": []any{map[string]any{"functionCall": map[string]any{
				"name": "lookup",
				"args": map[string]any{"query": "weather"},
			}}}},
			"finishReason": "STOP",
		}},
		"usageMetadata": map[string]any{"promptTokenCount": 2, "candidatesTokenCount": 3, "totalTokenCount": 5},
	}
	msg, usage, reason, err := m.readSSE(strings.NewReader(sseEvents(textEvent, toolEvent)), stream)
	if err != nil {
		t.Fatalf("readSSE: %v", err)
	}
	if reason != models.FinishReasonToolCalls {
		t.Fatalf("reason = %q, want tool_calls", reason)
	}
	if usage.TotalTokens != 5 {
		t.Fatalf("usage = %#v", usage)
	}
	if len(msg.Content) != 2 {
		t.Fatalf("content len = %d, want text and tool call", len(msg.Content))
	}
	if text, ok := msg.Content[0].(models.TextPart); !ok || text.Text != "hi" {
		t.Fatalf("text = %#v", msg.Content[0])
	}
	call, ok := msg.Content[1].(models.ToolCallPart)
	if !ok || call.Name != "lookup" || call.Input["query"] != "weather" {
		t.Fatalf("call = %#v", msg.Content[1])
	}
}

func TestAnthropicParsesReasoningDelta(t *testing.T) {
	m := &Model{Info_: models.ModelInfo{Provider: "anthropic", ID: "claude", API: models.APIAnthropicMessages}, Protocol: AnthropicMessages}
	stream := models.NewEventStream[models.StreamPart, *models.Message](16)
	msg, _, _, err := m.readSSE(strings.NewReader(sseEvents(
		map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "thinking_delta", "thinking": "think"}},
		map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}},
	)), stream)
	if err != nil {
		t.Fatalf("readSSE: %v", err)
	}
	found := false
	for {
		select {
		case part := <-stream.Iter():
			if delta, ok := part.(models.ReasoningDeltaPart); ok && delta.Delta == "think" {
				found = true
			}
		default:
			if !found {
				t.Fatal("reasoning delta not emitted")
			}
			if len(msg.Content) != 1 {
				t.Fatalf("content len = %d, want 1", len(msg.Content))
			}
			reasoning, ok := msg.Content[0].(models.ReasoningPart)
			if !ok || reasoning.Reasoning != "think" {
				t.Fatalf("reasoning = %#v", msg.Content[0])
			}
			return
		}
	}
}

func sseEvents(events ...map[string]any) string {
	var b strings.Builder
	for _, event := range events {
		b.WriteString("data: ")
		b.WriteString(mustJSON(event))
		b.WriteString("\n\n")
	}
	return b.String()
}

func mustJSON(v any) string {
	b, err := jsonMarshal(v)
	if err != nil {
		panic(fmt.Sprintf("marshal test event: %v", err))
	}
	return string(b)
}

var jsonMarshal = json.Marshal
