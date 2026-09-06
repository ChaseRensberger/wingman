package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
)

func TestProviderRoutesOnWire(t *testing.T) {
	for _, tt := range []struct {
		name                                        string
		api                                         models.API
		path, authHeader, authValue, terminal, body string
	}{
		{"responses", models.APIOpenAIResponses, "/responses", "Authorization", "Bearer key", `{"type":"response.completed"}`,
			`{"model":"test","stream":true,"input":[{"role":"system","content":"system"},{"role":"user","content":[{"type":"input_text","text":"hello"}]}],"max_output_tokens":17,"temperature":0.2,"top_p":0.8}`},
		{"chat", models.APIOpenAICompatible, "/chat/completions", "Authorization", "Bearer key", `{"choices":[{"finish_reason":"stop"}]}`,
			`{"model":"test","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"system","content":"system"},{"role":"user","content":"hello"}],"max_tokens":17,"temperature":0.2,"top_p":0.8}`},
		{"anthropic", models.APIAnthropicMessages, "/messages", "X-Api-Key", "key", `{"type":"message_stop"}`,
			`{"model":"test","stream":true,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],"system":[{"type":"text","text":"system"}],"max_tokens":17,"temperature":0.2,"top_p":0.8}`},
		{"gemini", models.APIGeminiGenerate, "/models/test:streamGenerateContent", "X-Goog-Api-Key", "key", `{"candidates":[{"finishReason":"STOP"}]}`,
			`{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"systemInstruction":{"parts":[{"text":"system"}]},"generationConfig":{"maxOutputTokens":17,"temperature":0.2,"topP":0.8}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, mode := range []string{"default", "custom auth", "no auth"} {
				t.Run(mode, func(t *testing.T) {
					var requests atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						requests.Add(1)
						if r.Method != "POST" || r.URL.Path != tt.path || r.Header.Get("Content-Type") != "application/json" {
							t.Errorf("request = %s %s, headers = %#v", r.Method, r.URL, r.Header)
						}
						if r.URL.Query().Get("configured") != "yes" || r.URL.Query().Get("same") != "request" || r.Header.Get("X-Request") != "yes" {
							t.Errorf("overrides missing: %s, %#v", r.URL, r.Header)
						}
						if tt.api == models.APIGeminiGenerate && r.URL.Query().Get("alt") != "sse" {
							t.Error("Gemini SSE query missing")
						}
						if tt.api == models.APIAnthropicMessages && r.Header.Get("Anthropic-Version") != "2023-06-01" {
							t.Error("Messages version missing")
						}
						switch mode {
						case "default":
							if r.Header.Get(tt.authHeader) != tt.authValue {
								t.Errorf("auth = %q", r.Header.Get(tt.authHeader))
							}
						case "custom auth":
							if r.Header.Get("X-Custom-Auth") != "Token key" || r.Header.Get(tt.authHeader) != "" {
								t.Errorf("custom auth = %#v", r.Header)
							}
						case "no auth":
							if r.Header.Get(tt.authHeader) != "" || r.Header.Get("X-Custom-Auth") != "" {
								t.Errorf("auth leaked: %#v", r.Header)
							}
						}
						var got, want map[string]any
						if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
							t.Error(err)
						}
						if err := json.Unmarshal([]byte(tt.body), &want); err != nil {
							t.Error(err)
						}
						if !reflect.DeepEqual(got, want) {
							t.Errorf("body = %#v, want %#v", got, want)
						}
						w.Header().Set("Content-Type", "text/event-stream")
						if _, err := w.Write([]byte("data: " + tt.terminal + "\n\n")); err != nil {
							t.Error(err)
						}
					}))
					defer upstream.Close()
					options := provider.ProviderOptions{BaseURL: upstream.URL, Query: map[string]string{"configured": "yes", "same": "configured"}}
					if mode == "custom auth" {
						options.AuthHeader, options.AuthScheme = "X-Custom-Auth", "Token"
					}
					if mode == "no auth" {
						auth := false
						options.Auth = &auth
					}
					registry, err := provider.NewRegistry(map[string]provider.ProviderConfig{"wire": {Options: options, Models: map[string]models.ModelInfo{"test": {API: tt.api}}}})
					if err != nil {
						t.Fatal(err)
					}
					temperature, topP := 0.2, 0.8
					_, err = registry.NewClient(map[string]string{"wire": "key"}).Generate(context.Background(), models.Request{
						Model: models.ModelRef{Provider: "wire", ID: "test"}, System: "system", Messages: []models.Message{models.NewUserText("hello")},
						Generation: models.Generation{MaxTokens: 17, Temperature: &temperature, TopP: &topP},
						HTTP:       models.HTTPOptions{Query: map[string]string{"same": "request"}, Headers: map[string]string{"x-request": "yes"}},
					})
					if err != nil {
						t.Fatal(err)
					}
					if requests.Load() != 1 {
						t.Fatalf("requests = %d", requests.Load())
					}
				})
			}
		})
	}
}
