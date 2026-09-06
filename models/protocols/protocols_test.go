package protocols

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/route"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func streamModel(protocol route.Protocol, body string) *route.Model {
	m := responsesTestModel(strings.NewReader(body))
	m.Route.Protocol = protocol
	return m
}

func prepareBody(t *testing.T, protocol route.Protocol, req models.Request) map[string]any {
	t.Helper()
	m := &route.Model{Info_: models.ModelInfo{ID: "test", Provider: "test"}, Route: route.Route{Protocol: protocol}}
	prepared, err := m.Prepare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return prepared.Body
}

func TestProtocolsCompletionFailureAndUsage(t *testing.T) {
	for _, tt := range []struct {
		name                     string
		protocol                 route.Protocol
		delta, terminal, failure string
		usage                    int
	}{
		{"responses", Responses{}, `{"type":"response.output_text.delta","delta":"hello"}`, `{"type":"response.completed","response":{"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`, `{"type":"response.failed","response":{"error":{"code":"server_error","message":"secret"}}}`, 3},
		{"chat", Chat{}, `{"choices":[{"delta":{"content":"hello"}}]}`, "{\"choices\":[{\"finish_reason\":\"stop\"}]}\n\ndata: {\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\ndata: [DONE]", `{"error":{"code":"server_error","message":"secret"}}`, 3},
		{"anthropic", Anthropic{}, `{"type":"content_block_delta","delta":{"text":"hello"}}`, "{\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":2}}}\n\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\ndata: {\"type\":\"message_stop\"}", `{"type":"error","error":{"type":"overloaded_error","message":"secret"}}`, 3},
		{"gemini", Gemini{}, `{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}`, `{"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`, `{"error":{"status":"UNAVAILABLE","message":"secret"}}`, 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, tc := range []struct {
				name, body string
				failed     bool
				text       string
			}{
				{"empty EOF", "", true, ""},
				{"partial EOF", "data: " + tt.delta + "\n\n", true, "hello"},
				{"failure", "data: " + tt.failure + "\n\n", true, ""},
				{"partial failure", "data: " + tt.delta + "\n\ndata: " + tt.failure + "\n\n", true, "hello"},
				{"empty completion", "data: " + tt.terminal + "\n\n", false, ""},
				{"completion", "data: " + tt.delta + "\n\ndata: " + tt.terminal + "\n\n", false, "hello"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					stream, err := streamModel(tt.protocol, tc.body).Stream(context.Background(), models.Request{})
					if err != nil {
						t.Fatal(err)
					}
					finishes, failures := 0, 0
					for part := range stream.Iter() {
						switch p := part.(type) {
						case models.FinishPart:
							finishes++
						case models.ErrorPart:
							failures++
							if strings.Contains(p.Error, "secret") {
								t.Fatal("raw error leaked")
							}
						}
					}
					msg, err := stream.Final()
					if (err != nil) != tc.failed || joinText(msg.Content) != tc.text {
						t.Fatalf("message = %#v, error = %v", msg, err)
					}
					if tc.failed {
						var failure *models.ProviderError
						if !errors.As(err, &failure) || failure.RequestID != "req_stream" || failure.Status != 200 {
							t.Fatalf("error = %#v", err)
						}
						if failures != 1 || finishes != 0 || msg.FinishReason != "" {
							t.Fatalf("failure lifecycle: %#v", msg)
						}
					} else if finishes != 1 || failures != 0 || msg.Usage == nil || msg.Usage.TotalTokens != tt.usage {
						t.Fatalf("completion lifecycle: %#v", msg)
					}
				})
			}
		})
	}
}

func TestNativeAndToolArgumentFailures(t *testing.T) {
	for _, tt := range []struct {
		name     string
		protocol route.Protocol
		events   []string
	}{
		{"responses native", Responses{}, []string{`{"type":"response.output_text.delta","delta":1}`}},
		{"chat native", Chat{}, []string{`{"choices":"invalid"}`}},
		{"anthropic native", Anthropic{}, []string{`{"type":"content_block_delta","index":"zero"}`}},
		{"gemini native", Gemini{}, []string{`{"candidates":"invalid"}`}},
		{"responses arguments", Responses{}, []string{`{"type":"response.output_item.done","item":{"type":"function_call","arguments":"{"}}`}},
		{"chat index", Chat{}, []string{`{"choices":[{"delta":{"tool_calls":[{"function":{}}]}}]}`}},
		{"chat arguments", Chat{}, []string{`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{"}}]}}]}`, `{"choices":[{"finish_reason":"tool_calls"}]}`}},
		{"anthropic arguments", Anthropic{}, []string{`{"type":"content_block_start","content_block":{"type":"tool_use"}}`, `{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{"}}`, `{"type":"content_block_stop"}`}},
		{"gemini arguments", Gemini{}, []string{`{"candidates":[{"content":{"parts":[{"functionCall":{"args":"not-object"}}]}}]}`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			parser := tt.protocol.NewParser(models.ModelInfo{Provider: "test"})
			var err error
			for _, event := range tt.events {
				_, err = parser.Step(route.Frame{Data: event})
				if err != nil {
					break
				}
			}
			var failure *models.ProviderError
			if !errors.As(err, &failure) || failure.Category != models.ErrorDecoding {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

func TestChatToolCallsUseProviderIndexOrder(t *testing.T) {
	parser := (Chat{}).NewParser(models.ModelInfo{})
	for _, event := range []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"second","function":{"name":"b","arguments":"{\"b\":"}},{"index":0,"id":"first","function":{"name":"a","arguments":"{\"a\":"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"2}"}},{"index":0,"function":{"arguments":"1}"}}]}}]}`,
		`{"choices":[{"finish_reason":"tool_calls"}]}`,
	} {
		if _, err := parser.Step(route.Frame{Data: event}); err != nil {
			t.Fatal(err)
		}
	}
	calls := toolCalls(parser.Message().Content)
	if len(calls) != 2 || calls[0].CallID != "first" || calls[1].CallID != "second" {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestChatChoiceAndPriorUsage(t *testing.T) {
	for _, event := range []string{`{"choices":[{"usage":{"prompt_tokens":120,"completion_tokens":30,"total_tokens":150}}]}`, `{"usage":{"prompt_tokens":120,"completion_tokens":30,"total_tokens":150}}`} {
		parser := (Chat{}).NewParser(models.ModelInfo{})
		for _, data := range []string{event, `{}`} {
			if _, err := parser.Step(route.Frame{Data: data}); err != nil {
				t.Fatal(err)
			}
		}
		usage := parser.Message().Usage
		if usage == nil || usage.InputTokens != 120 || usage.OutputTokens != 30 || usage.TotalTokens != 150 {
			t.Fatalf("usage = %#v", usage)
		}
	}
}

func TestCanonicalToolRequestBodies(t *testing.T) {
	req := models.Request{Messages: []models.Message{{Role: models.RoleAssistant, Content: models.Content{models.ToolPart{CallID: "call_1", Name: "lookup", State: models.ToolStateCompleted, Input: map[string]any{"query": "weather"}, Output: "sunny", OutputParts: models.Content{models.ImagePart{Base64: "image-data", MediaType: "image/png"}}, Structured: map[string]any{"answer": "42"}}}}}}
	chat := prepareBody(t, Chat{}, req)["messages"].([]any)
	if len(chat) != 2 || chat[0].(map[string]any)["role"] != "assistant" || chat[1].(map[string]any)["role"] != "tool" {
		t.Fatalf("messages = %#v", chat)
	}
	anthropic := prepareBody(t, Anthropic{}, req)["messages"].([]any)
	blocks := anthropic[1].(map[string]any)["content"].([]any)[0].(map[string]any)["content"].([]any)
	found := false
	for _, value := range blocks {
		block := value.(map[string]any)
		if block["type"] == "image" {
			found = block["source"].(map[string]any)["data"] == "image-data"
		}
	}
	if !found {
		t.Fatalf("image missing: %#v", blocks)
	}
	gemini := prepareBody(t, Gemini{}, req)["contents"].([]any)
	response := gemini[1].(map[string]any)["parts"].([]any)[0].(map[string]any)["functionResponse"].(map[string]any)["response"].(map[string]any)
	if response["structured"].(map[string]any)["answer"] != "42" {
		t.Fatalf("response = %#v", response)
	}
}

func TestThinkingAndReasoningReplay(t *testing.T) {
	req := models.Request{Capabilities: models.Capabilities{Thinking: true}}
	body := prepareBody(t, Responses{}, req)
	if body["reasoning"].(map[string]any)["summary"] != "auto" || body["include"].([]string)[0] != "reasoning.encrypted_content" {
		t.Fatalf("body = %#v", body)
	}
	if body := prepareBody(t, Anthropic{}, req); body["thinking"].(map[string]any)["type"] != "adaptive" {
		t.Fatalf("body = %#v", body)
	}
	for _, summary := range []string{"", "summary"} {
		parser := (Responses{}).NewParser(models.ModelInfo{})
		for _, event := range []string{`{"type":"response.reasoning_summary_text.delta","item_id":"rs_1","delta":"` + summary + `"}`, `{"type":"response.output_item.done","item":{"type":"reasoning","id":"rs_1","encrypted_content":"encrypted-state"}}`} {
			if _, err := parser.Step(route.Frame{Data: event}); err != nil {
				t.Fatal(err)
			}
		}
		msg := parser.Message()
		reason := msg.Content[0].(models.ReasoningPart)
		if reason.Reasoning != summary || reason.Encrypted != "encrypted-state" || reason.ProviderMetadata["openai"].(map[string]any)["item_id"] != "rs_1" {
			t.Fatalf("reason = %#v", reason)
		}
		msg.Content = append(msg.Content, models.ToolPart{CallID: "call_1", Name: "lookup", State: models.ToolStateCompleted, Input: map[string]any{}, Output: "sunny"})
		input := prepareBody(t, Responses{}, models.Request{Messages: []models.Message{*msg}})["input"].([]any)
		if len(input) != 3 || input[0].(map[string]any)["encrypted_content"] != "encrypted-state" || input[1].(map[string]any)["type"] != "function_call" || input[2].(map[string]any)["output"] != "sunny" {
			t.Fatalf("input = %#v", input)
		}
		if _, exists := input[0].(map[string]any)["id"]; exists {
			t.Fatal("stateless reasoning retained item ID")
		}
	}
}

func TestMultilineAndCancellation(t *testing.T) {
	msg, err := streamModel(Chat{}, "data: {\"choices\":[{\"delta\":{\n"+"data: \"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\n").Generate(context.Background(), models.Request{})
	if err != nil || joinText(msg.Content) != "hello" {
		t.Fatalf("message = %#v, error = %v", msg, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := streamModel(Chat{}, "").Stream(ctx, models.Request{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	reader := io.MultiReader(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"), afterCompletionReader{})
	model := responsesTestModel(reader)
	model.Route.Protocol = Chat{}
	stream, err := model.Stream(context.Background(), models.Request{})
	if err != nil {
		t.Fatal(err)
	}
	for range stream.Iter() {
	}
	msg, err = stream.Final()
	if err == nil || joinText(msg.Content) != "partial" {
		t.Fatalf("message = %#v, error = %v", msg, err)
	}
}
