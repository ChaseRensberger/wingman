package openaicompat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/protocols/openaichat"
)

func TestPrepareBuildsOpenAICompatibleChatRequest(t *testing.T) {
	c, err := New(Config{
		ProviderID: "test-compatible",
		APIKey:     "test-key",
		Model:      "test-model",
		BaseURL:    "https://example.test/v1",
		MaxTokens:  123,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	prepared, err := c.Prepare(context.Background(), models.Request{
		System: "system prompt",
		Messages: []models.Message{
			models.NewUserText("hello"),
		},
		Tools: []models.ToolDef{{
			Name:        "lookup",
			Description: "Look something up.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
			},
		}},
		ToolChoice: models.ToolChoice{Mode: models.ToolChoiceTool, Tool: "lookup"},
		OutputSchema: &models.OutputSchema{
			Name:   "answer",
			Schema: map[string]any{"type": "object"},
			Strict: true,
		},
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if prepared.URL != "https://example.test/v1/chat/completions" {
		t.Fatalf("url = %q", prepared.URL)
	}
	if got := prepared.Headers.Get("Authorization"); got != "Bearer test-key" {
		t.Fatalf("authorization = %q", got)
	}
	var body map[string]any
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["model"] != "test-model" {
		t.Fatalf("model = %#v", body["model"])
	}
	if body["stream"] != true {
		t.Fatalf("stream = %#v", body["stream"])
	}
	if body["max_tokens"] != float64(123) {
		t.Fatalf("max_tokens = %#v", body["max_tokens"])
	}
	if _, ok := body["tools"].([]any); !ok {
		t.Fatalf("tools missing or wrong type: %#v", body["tools"])
	}
	choice, ok := body["tool_choice"].(map[string]any)
	if !ok || choice["type"] != "function" {
		t.Fatalf("tool_choice = %#v", body["tool_choice"])
	}
	format, ok := body["response_format"].(map[string]any)
	if !ok || format["type"] != "json_schema" {
		t.Fatalf("response_format = %#v", body["response_format"])
	}
}

func TestPrepareAppliesChatProfileAndProviderOptions(t *testing.T) {
	c, err := New(Config{
		ProviderID: "test-compatible",
		APIKey:     "test-key",
		Model:      "test-model",
		BaseURL:    "https://example.test/v1",
		MaxTokens:  123,
		Profile:    mustKnownProfile(t, openaichat.ProfileOpenAIChat),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	prepared, err := c.Prepare(context.Background(), models.Request{
		System:   "system prompt",
		Messages: []models.Message{models.NewUserText("hello")},
		ProviderOptions: models.ProviderOptions{
			"test-compatible":                   {"parallel_tool_calls": false},
			string(models.APIOpenAICompletions): {"service_tier": "flex"},
		},
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := body["max_tokens"]; ok {
		t.Fatalf("max_tokens should be omitted for OpenAI Chat profile: %#v", body["max_tokens"])
	}
	if body["max_completion_tokens"] != float64(123) {
		t.Fatalf("max_completion_tokens = %#v", body["max_completion_tokens"])
	}
	if body["store"] != false {
		t.Fatalf("store = %#v", body["store"])
	}
	msgs, ok := body["messages"].([]any)
	if !ok || len(msgs) == 0 {
		t.Fatalf("messages = %#v", body["messages"])
	}
	first, ok := msgs[0].(map[string]any)
	if !ok || first["role"] != "developer" {
		t.Fatalf("first message = %#v", msgs[0])
	}
	if body["parallel_tool_calls"] != false {
		t.Fatalf("parallel_tool_calls = %#v", body["parallel_tool_calls"])
	}
	if body["service_tier"] != "flex" {
		t.Fatalf("service_tier = %#v", body["service_tier"])
	}
}

func mustKnownProfile(t *testing.T, id string) openaichat.Profile {
	t.Helper()
	profile, ok := openaichat.KnownProfile(id)
	if !ok {
		t.Fatalf("unknown profile %q", id)
	}
	return profile
}

func TestStreamUsesRouteTransportAndParsesEvents(t *testing.T) {
	var sawAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		sawAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"resp_1\",\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2,\"total_tokens\":6}}\n\n"))
	}))
	defer server.Close()

	c, err := New(Config{
		ProviderID: "test-compatible",
		APIKey:     "test-key",
		Model:      "test-model",
		BaseURL:    server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	stream, err := c.Stream(context.Background(), models.Request{Messages: []models.Message{models.NewUserText("hello")}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	for range stream.Iter() {
	}
	msg, err := stream.Final()
	if err != nil {
		t.Fatalf("Final() error = %v", err)
	}
	if sawAuth != "Bearer test-key" {
		t.Fatalf("authorization = %q", sawAuth)
	}
	if len(msg.Content) != 1 {
		t.Fatalf("content len = %d", len(msg.Content))
	}
	text, ok := msg.Content[0].(models.TextPart)
	if !ok || text.Text != "hi" {
		t.Fatalf("content = %#v", msg.Content[0])
	}
	if msg.FinishReason != models.FinishReasonStop {
		t.Fatalf("finish = %q", msg.FinishReason)
	}
}
