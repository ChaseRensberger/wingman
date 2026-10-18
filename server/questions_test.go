package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/question"
	"github.com/chaserensberger/wingman/store"
)

func TestQuestionRequestPathsAndRestart(t *testing.T) {
	for _, workflow := range []string{"production", "proxied-development", "vite-development"} {
		t.Run(workflow, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "questions.db")
			data, err := store.NewSQLiteStore(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = data.Close() }()
			owner, err := data.EnsureDefaultClient()
			if err != nil {
				t.Fatal(err)
			}
			other, err := data.CreateClient("other")
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			var sawAnswer atomic.Bool
			const explanation = "Choose a color before I continue."
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				if bytes.Contains(body, []byte(`"name":"question"`)) && !bytes.Contains(body, []byte(`"tool_call_id"`)) {
					arguments := `{"questions":[{"question":"Which color?","header":"Color","options":[{"label":"Blue","description":"Blue color"}]}]}`
					payload, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"role": "assistant", "content": explanation, "tool_calls": []any{map[string]any{"index": 0, "id": "call_question", "type": "function", "function": map[string]any{"name": "question", "arguments": arguments}}}}, "finish_reason": "tool_calls"}}})
					_, _ = io.WriteString(w, "data: "+string(payload)+"\n\ndata: [DONE]\n\n")
					return
				}
				if bytes.Contains(body, []byte(`"tool_call_id"`)) && bytes.Contains(body, []byte("Blue")) {
					sawAnswer.Store(true)
				}
				_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer upstream.Close()
			for _, agent := range []*store.Agent{
				{ID: "agt_question", Tools: []string{"question"}}, {ID: "agt_plain"},
			} {
				agent.ModelRef = "test/model"
				agent.Options = map[string]any{agentOptionModelRoute: models.ModelInfo{Provider: "test", ID: "model", API: models.APIOpenAICompatible, BaseURL: upstream.URL, Capabilities: models.ModelCapabilities{Tools: true}}}
				if err := data.CreateAgent(agent); err != nil {
					t.Fatal(err)
				}
			}
			for _, id := range []string{"ses_question", "ses_other", "ses_cancel"} {
				if err := data.CreateSession(&store.Session{ID: id, ClientID: owner.ID}); err != nil {
					t.Fatal(err)
				}
			}
			frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Errorf("API reached frontend: %s", r.URL.Path) }))
			defer frontend.Close()
			cfg := Config{Store: data, MaxActiveRuns: 1, MaxToolExecutions: 1, Username: "test", Password: "recovery-test-password", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
			if workflow == "proxied-development" {
				cfg.ConsoleDevURL = frontend.URL
			}
			server := New(cfg)
			if err := server.Start(ctx); err != nil {
				t.Fatal(err)
			}
			host := httptest.NewServer(server)
			defer func() { host.Close(); _ = server.Close(context.Background()) }()
			target := host.URL
			if workflow == "vite-development" {
				target = recoveryViteProxy(t, host.URL)
			}
			request := func(method, url string, input any, clientID string, auth bool) (int, []byte) {
				t.Helper()
				body, _ := json.Marshal(input)
				req, err := http.NewRequestWithContext(ctx, method, target+url, bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				if auth {
					req.SetBasicAuth("test", "recovery-test-password")
				}
				if clientID != "" {
					req.Header.Set("X-Wingman-Client", clientID)
				}
				response, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				output, _ := io.ReadAll(response.Body)
				return response.StatusCode, output
			}
			admit := func(sid, agent string) string {
				t.Helper()
				status, body := request("POST", "/sessions/"+sid+"/message", api.MessageSessionRequest{AgentID: agent, Message: "go"}, "", true)
				var admitted api.MessageSessionResponse
				if err := json.Unmarshal(body, &admitted); err != nil || status != 202 {
					t.Fatalf("admission=%d %s %v", status, body, err)
				}
				return admitted.RunID
			}
			await := func(sid, rid, status string) {
				t.Helper()
				for {
					run, err := data.GetSessionRun(ctx, sid, rid)
					if err != nil {
						t.Fatal(err)
					}
					if run.Status == status {
						return
					}
					if run.Status == "failed" {
						t.Fatalf("run failed: %#v", run)
					}
					select {
					case <-ctx.Done():
						t.Fatalf("await %s: %#v", status, run)
					case <-time.After(time.Millisecond):
					}
				}
			}
			runID := admit("ses_question", "agt_question")
			await("ses_question", runID, store.SessionRunStatusAwaitingInput)
			queuedID := admit("ses_question", "agt_plain")
			otherID := admit("ses_other", "agt_plain")
			await("ses_other", otherID, store.SessionRunStatusCompleted)
			queued, _ := data.GetSessionRun(ctx, "ses_question", queuedID)
			if queued.Status != "queued" || calls.Load() != 2 {
				t.Fatalf("wait held capacity or lost order: %#v calls=%d", queued, calls.Load())
			}
			status, body := request("GET", "/sessions/ses_question/events/history", nil, "", true)
			var page api.SessionEventPage
			if err := json.Unmarshal(body, &page); err != nil || status != http.StatusOK {
				t.Fatalf("history=%d %s %v", status, body, err)
			}
			foundExplanation := false
			for _, event := range page.Data {
				if message, ok := event.Data.(*api.MessageCreatedEventData); ok && message.RunID == runID {
					for _, part := range message.Message.Content {
						if text, ok := part.(models.TextPart); ok && text.Text == explanation {
							foundExplanation = message.Message.State == models.MessageStateInProgress
						}
					}
				}
			}
			if !foundExplanation {
				t.Fatal("suspended assistant explanation is missing from the event stream")
			}
			status, body = request("GET", "/sessions/ses_question/questions", nil, "", true)
			var questions []question.Question
			if err := json.Unmarshal(body, &questions); err != nil || status != 200 || len(questions) != 1 {
				t.Fatalf("questions=%d %s %v", status, body, err)
			}
			replyPath := "/sessions/ses_question/questions/" + questions[0].ID + "/reply"
			reply := api.QuestionReplyRequest{Status: "answered", Answers: [][]string{{"Blue"}}}
			if workflow != "vite-development" {
				if status, _ := request("POST", replyPath, reply, "", false); status != 401 {
					t.Fatalf("unauthenticated=%d", status)
				}
			}
			if status, _ := request("POST", replyPath, reply, other.ID, true); status != 403 {
				t.Fatalf("cross-client=%d", status)
			}
			if err := server.Close(ctx); err != nil {
				t.Fatal(err)
			}
			host.Close()
			if err := data.Close(); err != nil {
				t.Fatal(err)
			}
			data, err = store.NewSQLiteStore(path)
			if err != nil {
				t.Fatal(err)
			}
			cfg.Store = data
			server = New(cfg)
			if err := server.Start(ctx); err != nil {
				t.Fatal(err)
			}
			host = httptest.NewServer(server)
			target = host.URL
			if workflow == "vite-development" {
				target = recoveryViteProxy(t, host.URL)
			}
			await("ses_question", runID, store.SessionRunStatusAwaitingInput)
			if status, output := request("POST", replyPath, reply, "", true); status != 200 {
				t.Fatalf("reply=%d %s", status, output)
			}
			await("ses_question", runID, store.SessionRunStatusCompleted)
			await("ses_question", queuedID, store.SessionRunStatusCompleted)
			if !sawAnswer.Load() || calls.Load() != 4 {
				t.Fatalf("replayed provider or lost answer: calls=%d answer=%v", calls.Load(), sawAnswer.Load())
			}
			if status, _ := request("POST", replyPath, reply, "", true); status != 200 {
				t.Fatalf("retry=%d", status)
			}
			if status, _ := request("POST", replyPath, api.QuestionReplyRequest{Status: "dismissed"}, "", true); status != 409 {
				t.Fatalf("conflict=%d", status)
			}
			// Questions can also originate from an API caller without a tool or run.
			status, body = request("POST", "/sessions/ses_question/questions", api.CreateQuestionRequest{Questions: []question.Prompt{{Header: "API", Question: "Ready?", Options: []question.Choice{}}}}, "", true)
			var standalone question.Question
			if err := json.Unmarshal(body, &standalone); err != nil || status != 200 || standalone.RunID != "" {
				t.Fatalf("standalone=%d %s %v", status, body, err)
			}
			if status, _ := request("POST", "/sessions/ses_question/questions/"+standalone.ID+"/reply", api.QuestionReplyRequest{Status: "dismissed"}, "", true); status != 200 {
				t.Fatalf("dismiss=%d", status)
			}
			cancelID := admit("ses_cancel", "agt_question")
			await("ses_cancel", cancelID, store.SessionRunStatusAwaitingInput)
			if status, output := request("POST", "/sessions/ses_cancel/runs/"+cancelID+"/abort", nil, "", true); status != 200 {
				t.Fatalf("abort=%d %s", status, output)
			}
			status, body = request("GET", "/sessions/ses_cancel/questions", nil, "", true)
			if status != 200 || !strings.Contains(string(body), `"cancelled"`) {
				t.Fatalf("cancelled question missing: %s", body)
			}
		})
	}
}
