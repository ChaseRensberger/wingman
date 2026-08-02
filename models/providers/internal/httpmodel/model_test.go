package httpmodel

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/models"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestStreamEmitsProviderRequestID(t *testing.T) {
	tests := []struct {
		name      string
		headers   http.Header
		requestID string
	}{
		{
			name: "priority",
			headers: http.Header{
				"X-Request-Id":      {"preferred"},
				"Request-Id":        {"fallback"},
				"Openai-Request-Id": {"openai"},
				"X-Goog-Request-Id": {"google"},
			},
			requestID: "preferred",
		},
		{
			name:      "request-id fallback",
			headers:   http.Header{"Request-Id": {"anthropic"}},
			requestID: "anthropic",
		},
		{
			name:      "openai-request-id fallback",
			headers:   http.Header{"Openai-Request-Id": {"openai"}},
			requestID: "openai",
		},
		{
			name:      "x-goog-request-id fallback",
			headers:   http.Header{"X-Goog-Request-Id": {"google"}},
			requestID: "google",
		},
		{
			name:    "absent",
			headers: http.Header{"Cf-Ray": {"not-a-request-id"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &Model{
				Info_:    models.ModelInfo{Provider: "test", ID: "test"},
				Protocol: OpenAIChat,
				BaseURL:  "https://example.com",
				Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     tt.headers,
						Body:       io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
					}, nil
				})},
			}

			stream, err := model.Stream(context.Background(), models.Request{})
			if err != nil {
				t.Fatal(err)
			}
			var parts []models.StreamPart
			for part := range stream.Iter() {
				parts = append(parts, part)
			}
			if tt.requestID == "" {
				for _, part := range parts {
					if _, ok := part.(models.ResponseMetadataPart); ok {
						t.Fatalf("parts = %#v, want no response metadata", parts)
					}
				}
				return
			}
			if len(parts) == 0 {
				t.Fatal("parts = empty, want response metadata")
			}
			if _, ok := parts[0].(models.StreamStartPart); !ok {
				t.Fatalf("first part = %T, want StreamStartPart", parts[0])
			}
			if len(parts) < 2 {
				t.Fatalf("parts = %#v, want response metadata after stream start", parts)
			}
			metadata, ok := parts[1].(models.ResponseMetadataPart)
			if !ok {
				t.Fatalf("second part = %T, want ResponseMetadataPart", parts[1])
			}
			if len(metadata.Meta) != 1 || metadata.Meta["request_id"] != tt.requestID {
				t.Fatalf("metadata = %#v, want request_id %q", metadata.Meta, tt.requestID)
			}
		})
	}
}

func TestStreamErrorCarriesProviderRequestID(t *testing.T) {
	model := &Model{
		Info_:    models.ModelInfo{Provider: "test", ID: "test"},
		Protocol: OpenAIChat,
		BaseURL:  "https://example.com",
		Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"X-Request-Id": {"request-failed"}},
				Body:       io.NopCloser(strings.NewReader("rate limited")),
			}, nil
		})},
	}
	_, err := model.Stream(context.Background(), models.Request{})
	if err == nil {
		t.Fatal("Stream succeeded")
	}
	withRequestID, ok := err.(interface{ ProviderRequestID() string })
	if !ok || withRequestID.ProviderRequestID() != "request-failed" {
		t.Fatalf("error = %#v, want provider request ID", err)
	}
	var providerErr *models.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Category != models.ErrorRateLimit || !providerErr.Retryable {
		t.Fatalf("error = %#v, want retryable rate limit", err)
	}
}

func TestResponseErrorClassification(t *testing.T) {
	tests := []struct {
		status   int
		category models.ErrorCategory
		retry    bool
	}{
		{401, models.ErrorAuthentication, false}, {403, models.ErrorAuthorization, false}, {408, models.ErrorTimeout, true}, {429, models.ErrorRateLimit, true}, {400, models.ErrorInvalidRequest, false}, {503, models.ErrorUnavailable, true},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			err := responseError("test", &http.Response{StatusCode: tt.status, Header: http.Header{"Retry-After": {"2"}}})
			if err.Category != tt.category || err.Retryable != tt.retry || err.RequestID != "" {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

func TestRetryAfter(t *testing.T) {
	if d := retryAfter(http.Header{"Retry-After": {"2"}}); d == nil || *d != 2*time.Second {
		t.Fatalf("seconds retry after = %v", d)
	}
	if d := retryAfter(http.Header{"Retry-After": {time.Now().Add(time.Second).Format(http.TimeFormat)}}); d == nil || *d < 0 {
		t.Fatalf("date retry after = %v", d)
	}
}

func TestRequestOptionsAndQueryMerge(t *testing.T) {
	configured := map[string]string{"configured": "yes", "same": "old"}
	model := &Model{Info_: models.ModelInfo{Provider: "openai", ID: "test"}, Protocol: OpenAIChat, Route: &Route{Protocol: OpenAIChat, Endpoint: Endpoint{BaseURL: "https://example.com", Query: configured}}}
	req := models.Request{ProviderOptions: models.ProviderBag{"openai": {"temperature": 0.3, "model": "provider"}}, HTTP: models.HTTPOptions{Body: map[string]any{"model": "caller"}, Query: map[string]string{"same": "new", "request": "yes"}}}
	body, err := model.body(req)
	if err != nil || body["temperature"] != 0.3 || body["model"] != "caller" {
		t.Fatalf("body = %#v, err = %v", body, err)
	}
	route := model.route(req)
	if route.Endpoint.Query["same"] != "new" || route.Endpoint.Query["configured"] != "yes" || configured["same"] != "old" {
		t.Fatalf("query = %#v, configured = %#v", route.Endpoint.Query, configured)
	}
	gemini := mergeQuery(GeminiGenerate, map[string]string{"alt": "json"}, map[string]string{"request": "yes"})
	if gemini["alt"] != "json" || gemini["request"] != "yes" {
		t.Fatalf("Gemini query = %#v", gemini)
	}
}

func TestOpenAIChatToolCallsUseProviderIndexOrder(t *testing.T) {
	state := parseState{provider: "openai"}
	stream := models.NewEventStream[models.StreamPart, *models.Message](16)
	for _, event := range []map[string]any{
		{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": float64(1), "id": "second", "function": map[string]any{"name": "b", "arguments": `{"b":`}}, map[string]any{"index": float64(0), "id": "first", "function": map[string]any{"name": "a", "arguments": `{"a":`}}}}}}},
		{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": float64(1), "function": map[string]any{"arguments": `2}`}}, map[string]any{"index": float64(0), "function": map[string]any{"arguments": `1}`}}}}}}},
		{"choices": []any{map[string]any{"finish_reason": "tool_calls"}}},
	} {
		if err := parseOpenAIChat(event, &state, stream); err != nil {
			t.Fatal(err)
		}
	}
	if len(state.tools) != 2 || state.tools[0].CallID != "first" || state.tools[1].CallID != "second" {
		t.Fatalf("tool calls = %#v", state.tools)
	}
}

func TestToolArgumentFailuresAreDecodingErrors(t *testing.T) {
	tests := []struct {
		name string
		fn   func(*parseState, *models.EventStream[models.StreamPart, *models.Message]) error
	}{
		{"responses", func(s *parseState, stream *models.EventStream[models.StreamPart, *models.Message]) error {
			return parseOpenAIResponses(openAIResponsesEvent{Type: "response.output_item.done", Item: openAIResponsesOutputItem{Type: "function_call", Arguments: "{"}}, s, stream)
		}},
		{"chat missing index", func(s *parseState, stream *models.EventStream[models.StreamPart, *models.Message]) error {
			return parseOpenAIChat(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"function": map[string]any{}}}}}}}, s, stream)
		}},
		{"anthropic", func(s *parseState, stream *models.EventStream[models.StreamPart, *models.Message]) error {
			s.toolBuf = map[string]*toolAccum{"0": {args: *stringsBuilder("{")}}
			return parseAnthropic(anthropicEvent{Type: "content_block_stop"}, s, stream)
		}},
		{"gemini", func(s *parseState, stream *models.EventStream[models.StreamPart, *models.Message]) error {
			return (&Model{Info_: models.ModelInfo{Provider: "test"}, Protocol: GeminiGenerate}).handleSSEData(`{"candidates":[{"content":{"parts":[{"functionCall":{"args":"not-object"}}]}}]}`, s, stream)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn(&parseState{provider: "test"}, models.NewEventStream[models.StreamPart, *models.Message](16))
			var providerErr *models.ProviderError
			if !errors.As(err, &providerErr) || providerErr.Category != models.ErrorDecoding {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

func TestSSEMultilineAndIncompleteToolFailure(t *testing.T) {
	model := &Model{Info_: models.ModelInfo{Provider: "test"}, Protocol: OpenAIChat}
	stream := models.NewEventStream[models.StreamPart, *models.Message](16)
	message, _, _, err := model.readSSE(context.Background(), strings.NewReader("data: {\"choices\":[{\"delta\":{\n"+"data: \"content\":\"hello\"}}]}\n\n"), stream)
	if err != nil || len(message.Content) != 1 || message.Content[0].(models.TextPart).Text != "hello" {
		t.Fatalf("message = %#v, error = %v", message, err)
	}
	_, _, _, err = model.readSSE(context.Background(), strings.NewReader("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\"}}]}}]}\n\n"), models.NewEventStream[models.StreamPart, *models.Message](16))
	var providerErr *models.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Category != models.ErrorDecoding {
		t.Fatalf("error = %#v", err)
	}
}

func TestOpenAIChatSSERejectsInvalidNativeFields(t *testing.T) {
	model := &Model{Info_: models.ModelInfo{Provider: "test"}, Protocol: OpenAIChat}
	err := model.handleSSEData(`{"choices":"invalid"}`, &parseState{provider: "test"}, models.NewEventStream[models.StreamPart, *models.Message](1))
	var providerErr *models.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Category != models.ErrorDecoding {
		t.Fatalf("error = %#v", err)
	}
}

func TestNativeSSERejectsInvalidNativeFields(t *testing.T) {
	tests := []struct {
		name     string
		protocol Protocol
		data     string
	}{
		{"responses", OpenAIResponses, `{"type":"response.output_text.delta","delta":1}`},
		{"anthropic", AnthropicMessages, `{"type":"content_block_delta","index":"zero"}`},
		{"gemini", GeminiGenerate, `{"candidates":"invalid"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &Model{Info_: models.ModelInfo{Provider: "test"}, Protocol: tt.protocol}
			err := model.handleSSEData(tt.data, &parseState{provider: "test"}, models.NewEventStream[models.StreamPart, *models.Message](1))
			var providerErr *models.ProviderError
			if !errors.As(err, &providerErr) || providerErr.Category != models.ErrorDecoding {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

func stringsBuilder(value string) *strings.Builder {
	var builder strings.Builder
	builder.WriteString(value)
	return &builder
}

func TestParseOpenAIChatUsesChoiceUsage(t *testing.T) {
	state := parseState{}
	stream := models.NewEventStream[models.StreamPart, *models.Message](1)

	parseOpenAIChat(map[string]any{
		"choices": []any{
			map[string]any{
				"usage": map[string]any{
					"prompt_tokens":     float64(120),
					"completion_tokens": float64(30),
					"total_tokens":      float64(150),
				},
			},
		},
	}, &state, stream)

	if state.usage.InputTokens != 120 || state.usage.OutputTokens != 30 || state.usage.TotalTokens != 150 {
		t.Fatalf("usage = %#v, want usage from choices[0]", state.usage)
	}
}

func TestParseOpenAIChatKeepsPriorUsage(t *testing.T) {
	state := parseState{}
	stream := models.NewEventStream[models.StreamPart, *models.Message](1)

	parseOpenAIChat(map[string]any{
		"usage": map[string]any{
			"prompt_tokens":     float64(120),
			"completion_tokens": float64(30),
			"total_tokens":      float64(150),
		},
	}, &state, stream)
	parseOpenAIChat(map[string]any{}, &state, stream)

	if state.usage.TotalTokens != 150 {
		t.Fatalf("total tokens = %d, want prior usage retained", state.usage.TotalTokens)
	}
}

func TestOpenAIChatBodyExpandsCanonicalToolPart(t *testing.T) {
	model := &Model{Protocol: OpenAIChat, Info_: models.ModelInfo{ID: "test"}}
	body, err := model.body(models.Request{Messages: []models.Message{{
		Role: models.RoleAssistant,
		Content: models.Content{models.ToolPart{
			CallID: "call_1",
			Name:   "bash",
			State:  models.ToolStateCompleted,
			Input:  map[string]any{"command": "pwd"},
			Output: "/tmp",
		}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	messages := body["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages = %#v, want assistant tool call and tool result", messages)
	}
	if messages[0].(map[string]any)["role"] != "assistant" || messages[1].(map[string]any)["role"] != "tool" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestOpenAIResponsesBodyRequestsReasoningSummary(t *testing.T) {
	model := &Model{Protocol: OpenAIResponses, Info_: models.ModelInfo{ID: "test"}}
	body, err := model.body(models.Request{Capabilities: models.Capabilities{Thinking: true}})
	if err != nil {
		t.Fatal(err)
	}
	if reasoning, ok := body["reasoning"].(map[string]any); !ok || reasoning["summary"] != "auto" {
		t.Fatalf("reasoning = %#v, want automatic summary", body["reasoning"])
	}
	if include, ok := body["include"].([]string); !ok || len(include) != 1 || include[0] != "reasoning.encrypted_content" {
		t.Fatalf("include = %#v, want encrypted reasoning", body["include"])
	}
}

func TestAnthropicBodyUsesAdaptiveThinking(t *testing.T) {
	model := &Model{Protocol: AnthropicMessages, Info_: models.ModelInfo{ID: "test"}}
	body, err := model.body(models.Request{Capabilities: models.Capabilities{Thinking: true}})
	if err != nil {
		t.Fatal(err)
	}
	if thinking, ok := body["thinking"].(map[string]any); !ok || thinking["type"] != "adaptive" {
		t.Fatalf("thinking = %#v, want adaptive thinking", body["thinking"])
	}
}

func TestParseOpenAIResponsesKeepsReasoningSummary(t *testing.T) {
	state := parseState{}
	stream := models.NewEventStream[models.StreamPart, *models.Message](2)
	parseOpenAIResponses(openAIResponsesEvent{Type: "response.reasoning_summary_text.delta", ItemID: "rs_1", Delta: "**Planning lookup**"}, &state, stream)
	parseOpenAIResponses(openAIResponsesEvent{Type: "response.output_item.done", Item: openAIResponsesOutputItem{Type: "reasoning", ID: "rs_1", EncryptedContent: "encrypted-state"}}, &state, stream)
	if got := state.message().Content; len(got) != 1 {
		t.Fatalf("content = %#v, want reasoning part", got)
	} else if reasoning, ok := got[0].(models.ReasoningPart); !ok || reasoning.Reasoning != "**Planning lookup**" || reasoning.Encrypted != "encrypted-state" {
		t.Fatalf("content = %#v", got)
	}
}

func TestOpenAIResponsesBodyRoundsTripEncryptedReasoning(t *testing.T) {
	model := &Model{Protocol: OpenAIResponses, Info_: models.ModelInfo{ID: "test"}}
	body, err := model.body(models.Request{Messages: []models.Message{{
		Role: models.RoleAssistant,
		Content: models.Content{models.ReasoningPart{
			Reasoning:        "summary",
			Encrypted:        "encrypted-state",
			ProviderMetadata: models.Meta{"openai": map[string]any{"item_id": "rs_1", "reasoning_encrypted_content": "encrypted-state"}},
		}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	input := body["input"].([]any)
	if len(input) != 1 {
		t.Fatalf("input = %#v", input)
	}
	reasoning := input[0].(map[string]any)
	if reasoning["type"] != "reasoning" || reasoning["id"] != "rs_1" || reasoning["encrypted_content"] != "encrypted-state" {
		t.Fatalf("reasoning = %#v", reasoning)
	}
}
