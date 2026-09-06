package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestProviderStreamFailureReachesSessionAPI(t *testing.T) {
	for _, tt := range []struct {
		name, body, category, message string
		api                           models.API
	}{
		{"responses failed", "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"server_error\",\"message\":\"private provider detail\"}}}\n\n", "unavailable", "provider is unavailable", models.APIOpenAIResponses},
		{"responses error", "data: {\"type\":\"error\",\"code\":\"rate_limit_exceeded\",\"message\":\"private provider detail\"}\n\n", "rate_limit", "provider rate limit exceeded", models.APIOpenAIResponses},
		{"responses unterminated", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n", "decoding", "provider response ended without a completion event", models.APIOpenAIResponses},
		{"chat failed", "data: {\"error\":{\"code\":\"server_error\",\"message\":\"private provider detail\"}}\n\n", "unavailable", "provider is unavailable", models.APIOpenAICompatible},
		{"chat unterminated", "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n", "decoding", "provider response ended without a completion event", models.APIOpenAICompatible},
		{"anthropic failed", "data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"private provider detail\"}}\n\n", "unavailable", "provider is unavailable", models.APIAnthropicMessages},
		{"anthropic unterminated", "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"partial\"}}\n\n", "decoding", "provider response ended without a completion event", models.APIAnthropicMessages},
		{"gemini failed", "data: {\"error\":{\"status\":\"UNAVAILABLE\",\"message\":\"private provider detail\"}}\n\n", "unavailable", "provider is unavailable", models.APIGeminiGenerate},
		{"gemini unterminated", "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"partial\"}]}}]}\n\n", "decoding", "provider response ended without a completion event", models.APIGeminiGenerate},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var requests atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("X-Request-ID", "req_failure")
				if _, err := io.WriteString(w, tt.body); err != nil {
					t.Error(err)
				}
			}))
			defer upstream.Close()
			data := memory.NewStore()
			owner, err := data.EnsureDefaultClient()
			if err != nil {
				t.Fatal(err)
			}
			const sid = "ses_provider_failure"
			if err := data.CreateSession(&store.Session{ID: sid, ClientID: owner.ID}); err != nil {
				t.Fatal(err)
			}
			agent := &store.Agent{
				ID: "agt_provider_failure", Name: "Assist", ModelRef: "test/responses",
				Options: map[string]any{agentOptionModelRoute: models.ModelInfo{
					Provider: "test", ID: "responses", API: tt.api, BaseURL: upstream.URL,
				}},
			}
			if err := data.CreateAgent(agent); err != nil {
				t.Fatal(err)
			}
			server := New(Config{
				Store: data, GlobalInstructionsPath: filepath.Join(t.TempDir(), "AGENTS.md"),
				GlobalSkillDirs: []string{t.TempDir()},
			})
			defer func() {
				if err := server.Close(context.Background()); err != nil {
					t.Error(err)
				}
			}()
			request := httptest.NewRequest(http.MethodPost, "/sessions/"+sid+"/message", strings.NewReader(`{"agent_id":"agt_provider_failure","message":"how are you?"}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			server.router.ServeHTTP(response, request)
			if response.Code != http.StatusAccepted {
				t.Fatalf("admission = %d: %s", response.Code, response.Body.String())
			}
			var admission api.MessageSessionResponse
			if err := json.Unmarshal(response.Body.Bytes(), &admission); err != nil {
				t.Fatal(err)
			}
			deadline := time.After(5 * time.Second)
			for {
				run, err := data.GetSessionRun(context.Background(), sid, admission.RunID)
				if err != nil {
					t.Fatal(err)
				}
				if run.Status != store.SessionRunStatusRunning && run.Status != store.SessionRunStatusQueued {
					if run.Status != store.SessionRunStatusFailed || !strings.Contains(run.ErrorMessage, tt.message) {
						t.Fatalf("run = %#v", run)
					}
					break
				}
				select {
				case <-deadline:
					t.Fatal("timed out waiting for failed run")
				case <-time.After(time.Millisecond):
				}
			}
			calls, err := data.ListModelCalls(context.Background(), sid)
			if err != nil || len(calls) != 1 {
				t.Fatalf("model calls = %#v, error = %v", calls, err)
			}
			call := calls[0]
			if call.Status != store.ModelCallStatusFailed || call.ErrorType != tt.category || call.ProviderRequestID != "req_failure" || call.FinishReason != "" {
				t.Fatalf("model call = %#v", call)
			}
			if requests.Load() != 1 {
				t.Fatalf("provider dispatched %d times; stream failures must not be retried", requests.Load())
			}
			events, err := data.ListSessionEvents(context.Background(), sid, 0, 100)
			if err != nil || len(events) == 0 {
				t.Fatalf("events = %#v, error = %v", events, err)
			}
			terminal := events[len(events)-1]
			if terminal.Type != "session.run.failed" || !strings.Contains(string(terminal.Data), tt.message) || strings.Contains(string(terminal.Data), "private provider detail") {
				t.Fatalf("terminal event = %#v", terminal)
			}
			for _, path := range []string{"/sessions/" + sid, "/sessions/" + sid + "/runs/" + admission.RunID, "/sessions/" + sid + "/model-calls"} {
				response := httptest.NewRecorder()
				server.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
				if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "private provider detail") {
					t.Fatalf("%s = %d: %s", path, response.Code, response.Body.String())
				}
			}
			detail := httptest.NewRecorder()
			server.router.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "/sessions/"+sid, nil))
			var session api.SessionDetail
			if err := json.Unmarshal(detail.Body.Bytes(), &session); err != nil {
				t.Fatal(err)
			}
			if len(session.History) != 2 || session.History[1].State != models.MessageStateFailed {
				t.Fatalf("history = %#v, want failed assistant", session.History)
			}
			if strings.Contains(tt.name, "unterminated") {
				content := session.History[1].Content
				if len(content) != 1 || content[0].(models.TextPart).Text != "partial" {
					t.Fatalf("partial output lost: %#v", content)
				}
			}
		})
	}
}
