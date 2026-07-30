package httpmodel

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

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
	parseOpenAIResponses(map[string]any{
		"type":    "response.reasoning_summary_text.delta",
		"item_id": "rs_1",
		"delta":   "**Planning lookup**",
	}, &state, stream)
	parseOpenAIResponses(map[string]any{
		"type": "response.output_item.done",
		"item": map[string]any{"type": "reasoning", "id": "rs_1", "encrypted_content": "encrypted-state"},
	}, &state, stream)
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
