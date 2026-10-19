package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/plugins/compaction"
	"github.com/chaserensberger/wingman/store"
)

func TestCompactionUsageRequestPathsAndRestart(t *testing.T) {
	for _, workflow := range []string{"production", "proxied-development", "vite-development"} {
		t.Run(workflow, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "compaction.db")
			data, err := store.NewSQLiteStore(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = data.Close() }()
			owner, err := data.EnsureDefaultClient()
			if err != nil {
				t.Fatal(err)
			}
			if err := data.CreateSession(&store.Session{ID: "ses_compaction", ClientID: owner.ID}); err != nil {
				t.Fatal(err)
			}
			for i, role := range []string{"user", "assistant"} {
				id := "msg_compaction_" + role
				if err := data.SaveMessage(ctx, store.StoredMessage{
					ID: id, SessionID: "ses_compaction", Idx: i, Role: role, Revision: 1, State: "completed",
					Parts: []store.StoredPart{{ID: "part_" + role, MessageID: id, Sequence: 0, Kind: "text", PayloadJSON: []byte(`{"type":"text","text":"Keep this conversation"}`)}},
				}); err != nil {
					t.Fatal(err)
				}
			}
			started, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			resume := func() { releaseOnce.Do(func() { close(release) }) }
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"The complete summary\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":60300,\"completion_tokens\":1900,\"total_tokens\":62200}}\n\ndata: [DONE]\n\n")
			}))
			defer upstream.Close()
			defer resume()
			if err := data.CreateAgent(&store.Agent{
				ID: "agt_compaction", ModelRef: "test/model",
				Options: map[string]any{agentOptionModelRoute: models.ModelInfo{Provider: "test", ID: "model", API: models.APIOpenAICompatible, BaseURL: upstream.URL, ContextWindow: 4096}},
			}); err != nil {
				t.Fatal(err)
			}
			frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("API reached frontend: %s", r.URL.Path)
			}))
			defer frontend.Close()
			for phase := 0; phase < 2; phase++ {
				cfg := Config{
					Store: data, Username: "test", Password: "recovery-test-password",
					Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
					Plugins: []PluginRegistration{{ID: "compaction", Enabled: true, Factory: func() plugin.Plugin { return compaction.New() }}},
				}
				if workflow == "proxied-development" {
					cfg.ConsoleDevURL = frontend.URL
				}
				server := New(cfg)
				if err := server.Start(ctx); err != nil {
					t.Fatal(err)
				}
				host := httptest.NewServer(server)
				t.Cleanup(func() { host.Close(); _ = server.Close(context.Background()) })
				target := host.URL
				if workflow == "vite-development" {
					target = recoveryViteProxy(t, host.URL)
				}
				request := func(method, path, body string, output any) {
					t.Helper()
					req, err := http.NewRequestWithContext(ctx, method, target+path, strings.NewReader(body))
					if err != nil {
						t.Fatal(err)
					}
					req.SetBasicAuth("test", "recovery-test-password")
					req.Header.Set("Content-Type", "application/json")
					response, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatal(err)
					}
					defer response.Body.Close()
					if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted {
						body, _ := io.ReadAll(response.Body)
						t.Fatalf("%s %s: %d %s", method, path, response.StatusCode, body)
					}
					if err := json.NewDecoder(response.Body).Decode(output); err != nil {
						t.Fatal(err)
					}
				}
				if phase == 0 {
					var admission api.ActionSessionResponse
					request("POST", "/sessions/ses_compaction/actions/compaction.compact", `{"agent_id":"agt_compaction"}`, &admission)
					select {
					case <-started:
					case <-ctx.Done():
						t.Fatal("summarization request did not start")
					}
					var running api.SessionDetail
					request("GET", "/sessions/ses_compaction", "", &running)
					if len(running.History) != 3 || running.History[2].State != models.MessageStateInProgress || running.History[2].Revision != 1 {
						t.Fatalf("running compaction = %#v", running.History)
					}
					resume()
					for {
						run, err := data.GetSessionRun(ctx, "ses_compaction", admission.RunID)
						if err != nil {
							t.Fatal(err)
						}
						if run.Status == store.SessionRunStatusCompleted {
							break
						}
						if run.Status == store.SessionRunStatusFailed || ctx.Err() != nil {
							t.Fatalf("compaction status=%s error=%s: %s", run.Status, run.ErrorType, run.ErrorMessage)
						}
						time.Sleep(time.Millisecond)
					}
				}
				var detail api.SessionDetail
				request("GET", "/sessions/ses_compaction", "", &detail)
				if len(detail.History) != 3 || detail.History[2].State != models.MessageStateCompleted || detail.History[2].Revision != 2 {
					t.Fatalf("phase=%d completed compaction = %#v", phase, detail.History)
				}
				var marker compaction.MarkerPart
				for _, message := range detail.History {
					for _, part := range message.Content {
						if value, ok := compaction.DecodeMarker(part); ok {
							marker = value
						}
					}
				}
				if marker.Summary != "The complete summary" || marker.Usage == nil || marker.Usage.InputTokens != 60300 || marker.Usage.OutputTokens != 1900 {
					t.Fatalf("phase=%d checkpoint=%#v", phase, marker)
				}
				var events api.SessionEventPage
				request("GET", "/sessions/ses_compaction/events/history", "", &events)
				found := false
				foundRunning := false
				for _, event := range events.Data {
					if created, ok := event.Data.(*api.MessageCreatedEventData); ok {
						if created.Message.ID == detail.History[2].ID && created.Message.State == models.MessageStateInProgress {
							foundRunning = true
						}
						for _, part := range created.Message.Content {
							if value, ok := compaction.DecodeMarker(part); ok && value.Usage != nil && *value.Usage == *marker.Usage {
								found = true
							}
						}
					}
				}
				if !found || !foundRunning {
					t.Fatalf("phase=%d checkpoint usage missing from session events", phase)
				}
				host.Close()
				if err := server.Close(ctx); err != nil {
					t.Fatal(err)
				}
				if phase == 0 {
					if err := data.Close(); err != nil {
						t.Fatal(err)
					}
					data, err = store.NewSQLiteStore(path)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}
