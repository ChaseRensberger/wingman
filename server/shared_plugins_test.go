package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestSharedPluginActionRequestPaths(t *testing.T) {
	for _, workflow := range []string{"production", "proxied-development", "vite-development"} {
		t.Run(workflow, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
			defer cancel()
			data := memory.NewStore()
			owner, err := data.EnsureDefaultClient()
			if err != nil {
				t.Fatal(err)
			}
			if err := data.CreateSession(&store.Session{ID: "ses_agent_action", ClientID: owner.ID}); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"first", "second"} {
				agent := &store.Agent{
					ID: id, Name: id, ModelRef: "test/model",
					Options: map[string]any{agentOptionModelRoute: models.ModelInfo{Provider: "test", ID: "model", API: models.APIOpenAICompatible, BaseURL: "http://127.0.0.1:1"}},
				}
				if err := data.CreateAgent(agent); err != nil {
					t.Fatal(err)
				}
			}
			frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("API reached frontend: %s", r.URL.Path)
			}))
			defer frontend.Close()
			cfg := Config{
				Store: data, Username: "test", Password: "recovery-test-password",
				Plugins: []PluginRegistration{{ID: "test", Enabled: true, Factory: func() plugin.Plugin { return sessionActionTestPlugin{} }}},
			}
			if workflow == "proxied-development" {
				cfg.ConsoleDevURL = frontend.URL
			}
			s := New(cfg)
			defer s.Close(context.Background())
			if err := s.Start(ctx); err != nil {
				t.Fatal(err)
			}
			host := httptest.NewServer(s)
			defer host.Close()
			target := host.URL
			if workflow == "vite-development" {
				target = recoveryViteProxy(t, host.URL)
			}
			admit := func(action, agent string, want int) api.ActionSessionResponse {
				t.Helper()
				body := `{"request_id":"agent-action-` + agent + `","agent_id":"` + agent + `"}`
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, target+"/sessions/ses_agent_action/actions/"+action, strings.NewReader(body))
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
				if response.StatusCode != want {
					body, _ := io.ReadAll(response.Body)
					t.Fatalf("action=%s agent=%s: status=%d want=%d body=%s", action, agent, response.StatusCode, want, body)
				}
				var admission api.ActionSessionResponse
				if want == http.StatusAccepted {
					if err := json.NewDecoder(response.Body).Decode(&admission); err != nil {
						t.Fatal(err)
					}
				}
				return admission
			}
			admit("missing", "first", http.StatusNotFound)
			for _, agent := range []string{"first", "second"} {
				first := admit("run", agent, http.StatusAccepted)
				retry := admit("test.run", agent, http.StatusAccepted)
				if first.RunID == "" || retry.RunID != first.RunID {
					t.Fatalf("action retry created a different run: first=%+v retry=%+v", first, retry)
				}
				for {
					run, err := data.GetSessionRun(ctx, "ses_agent_action", first.RunID)
					if err != nil {
						t.Fatal(err)
					}
					if run.Action != "test.run" {
						t.Fatalf("action alias was not canonicalized: %s", run.Action)
					}
					if run.Status == store.SessionRunStatusCompleted {
						break
					}
					if run.Status == store.SessionRunStatusFailed || ctx.Err() != nil {
						t.Fatalf("action status=%s error=%s: %s", run.Status, run.ErrorType, run.ErrorMessage)
					}
					time.Sleep(time.Millisecond)
				}
			}
		})
	}
}
