package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestSessionQueueRequestPaths(t *testing.T) {
	frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/console/" {
			t.Errorf("API request reached frontend: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("Console development frontend"))
	}))
	defer frontend.Close()
	for _, devURL := range []string{"", frontend.URL} {
		name := "production"
		if devURL != "" {
			name = "proxied development"
		}
		t.Run(name, func(t *testing.T) {
			data := memory.NewStore()
			owner, err := data.EnsureDefaultClient()
			if err != nil {
				t.Fatal(err)
			}
			if err := data.CreateSession(&store.Session{ID: "s", ClientID: owner.ID}); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"plan", "build"} {
				if err := data.CreateAgent(&store.Agent{ID: id, Name: id, ModelRef: "test/model", Options: map[string]any{agentOptionModelRoute: models.ModelInfo{Provider: "test", ID: "model", API: models.APIOpenAICompatible, BaseURL: "http://127.0.0.1:1"}}}); err != nil {
					t.Fatal(err)
				}
			}
			s := New(Config{Store: &admissionTestStore{Store: data}, ConsoleDevURL: devURL, Username: "queue", Password: "test-password", Plugins: []PluginRegistration{
				{ID: "test", Enabled: true, Factory: func() plugin.Plugin { return sessionActionTestPlugin{} }},
			}})
			defer s.Close(t.Context())
			request := func(method, path, body string, authorized bool) *httptest.ResponseRecorder {
				t.Helper()
				r := httptest.NewRequest(method, path, strings.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
				if authorized {
					r.SetBasicAuth("queue", "test-password")
				}
				w := httptest.NewRecorder()
				s.ServeHTTP(w, r)
				return w
			}
			admit := func(agent string) api.MessageSessionResponse {
				t.Helper()
				body := `{"request_id":"` + agent + `","agent_id":"` + agent + `","message":"same"}`
				if w := request(http.MethodPost, "/sessions/s/message", body, false); w.Code != http.StatusUnauthorized {
					t.Fatalf("unauthorized admission = %d", w.Code)
				}
				w := request(http.MethodPost, "/sessions/s/message", body, true)
				var result api.MessageSessionResponse
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != http.StatusAccepted {
					t.Fatalf("admission = %s, %v", w.Body.String(), err)
				}
				return result
			}
			first := admit("plan")
			if _, err := data.ClaimNextSessionRun(t.Context(), "s"); err != nil {
				t.Fatal(err)
			}
			second := admit("build")
			if second.Status != "queued" {
				t.Fatalf("second status = %s", second.Status)
			}
			if retry := admit("build"); retry.RunID != second.RunID {
				t.Fatalf("retry duplicated run: %#v", retry)
			}
			w := request(http.MethodGet, "/sessions/s/runs", "", true)
			var runs []api.SessionRun
			if err := json.Unmarshal(w.Body.Bytes(), &runs); err != nil || w.Code != http.StatusOK || len(runs) != 2 || runs[0].Status != "running" || runs[1].Status != "queued" {
				t.Fatalf("runs = %s, %v", w.Body.String(), err)
			}
			if _, err := data.SettleSessionRun(t.Context(), store.SessionRunSettlement{ID: first.RunID, ExpectedStatus: "running", Status: "completed"}); err != nil {
				t.Fatal(err)
			}
			if _, err := data.ClaimNextSessionRun(t.Context(), "s"); err != nil {
				t.Fatal(err)
			}
			if err := data.SaveMessage(t.Context(), store.StoredMessage{ID: "m", SessionID: "s", RunID: second.RunID, Role: "user", State: "completed", Revision: 1}); err != nil {
				t.Fatal(err)
			}
			w = request(http.MethodGet, "/sessions/s", "", true)
			var detail api.SessionDetail
			if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil || w.Code != http.StatusOK || len(detail.Notices) != 1 {
				t.Fatalf("detail = %s, %v", w.Body.String(), err)
			}
			change, ok := detail.Notices[0].Data.(*api.AgentSwitchedEventData)
			if !ok || change.Agent != "build" || change.Previous != "plan" || change.RunID != second.RunID {
				t.Fatalf("change = %#v", detail.Notices[0])
			}
			if len(detail.History) != 1 || detail.History[0].Metadata["run_id"] != second.RunID {
				t.Fatalf("history = %#v", detail.History)
			}
			w = request(http.MethodGet, "/sessions/s/events/history", "", true)
			var page api.SessionEventPage
			if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, event := range page.Data {
				if event.ID == detail.Notices[0].ID {
					found = true
				}
			}
			if !found {
				t.Fatal("switch notice is missing from durable event history")
			}
			for _, path := range []string{"/sessions/s", "/sessions/s/runs", "/sessions/s/events/history"} {
				if w := request(http.MethodGet, path, "", false); w.Code != http.StatusUnauthorized {
					t.Fatalf("unauthorized %s = %d", path, w.Code)
				}
			}
			w = request(http.MethodGet, "/console/", "", true)
			if w.Code != http.StatusOK {
				t.Fatalf("console = %d: %s", w.Code, w.Body.String())
			}
			if devURL != "" && w.Body.String() != "Console development frontend" {
				t.Fatalf("wrong frontend: %s", w.Body.String())
			}
			const actionBody = `{"request_id":"action-retry","agent_id":"build","model_ref":"test/model","input":{"arguments":"keep recent changes","limit":10}}`
			if w := request(http.MethodPost, "/sessions/s/actions/test.run", actionBody, false); w.Code != http.StatusUnauthorized {
				t.Fatalf("unauthorized action = %d", w.Code)
			}
			w = request(http.MethodPost, "/sessions/s/actions/test.run", actionBody, true)
			var action api.ActionSessionResponse
			if err := json.Unmarshal(w.Body.Bytes(), &action); err != nil || w.Code != http.StatusAccepted {
				t.Fatalf("action admission = %s, %v", w.Body.String(), err)
			}
			stored, err := data.GetSessionRun(t.Context(), "s", action.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Action != "test.run" || string(stored.InputJSON) != `{"arguments":"keep recent changes","limit":10}` {
				t.Fatalf("action retry input = %#v", stored)
			}
		})
	}
}
