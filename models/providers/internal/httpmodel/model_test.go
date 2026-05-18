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
