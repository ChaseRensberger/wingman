package server

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/chaserensberger/wingman/trigger"
)

func triggerTestServer(t *testing.T, cfg Config) (*Server, *store.SQLiteStore, string) {
	t.Helper()
	data, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "triggers.db"))
	if err != nil {
		t.Fatal(err)
	}
	owner, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	agent := &store.Agent{ID: "agt_trigger", Name: "Trigger test", ModelRef: "test/model", Options: map[string]any{agentOptionModelRoute: models.ModelInfo{Provider: "test", ID: "model", API: models.APIOpenAICompatible, BaseURL: "http://127.0.0.1:1"}}}
	if err := data.CreateAgent(agent); err != nil {
		t.Fatal(err)
	}
	cfg.Store, cfg.GlobalInstructionsPath, cfg.GlobalSkillDirs = data, filepath.Join(t.TempDir(), "AGENTS.md"), []string{t.TempDir()}
	s := New(cfg)
	t.Cleanup(func() {
		if err := s.Close(context.Background()); err != nil {
			t.Error(err)
		}
		if err := data.Close(); err != nil {
			t.Error(err)
		}
	})
	return s, data, owner.ID
}

func testTriggerDefinition() trigger.Config {
	return trigger.Config{Name: "Review", Prompt: "Check the project", Enabled: true, Source: trigger.Source{Type: "cron", Expression: "0 8 * * 1-5", TimeZone: "America/New_York"}, Target: trigger.Target{AgentID: "agt_trigger"}}
}

func triggerRequest(t *testing.T, s *Server, method, path, clientID string, body any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(method, path, strings.NewReader(string(encoded)))
	r.Header.Set("Content-Type", "application/json")
	if clientID != "" {
		r.Header.Set("X-Wingman-Client", clientID)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestTriggerHTTPCRUDAndIsolation(t *testing.T) {
	s, data, owner := triggerTestServer(t, Config{})
	s.runs.stop()
	created := triggerRequest(t, s, "POST", "/triggers", owner, testTriggerDefinition())
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var item trigger.Trigger
	if err := json.Unmarshal(created.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.NextFireAt == nil || item.Version != 1 || item.ClientID != owner {
		t.Fatalf("trigger = %#v", item)
	}

	other, err := data.CreateClient("Another client")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct {
		method, suffix string
		body           any
	}{
		{"GET", "", nil}, {"PUT", "", api.UpdateTriggerRequest{Config: testTriggerDefinition(), ExpectedVersion: 1}},
		{"PUT", "/state", api.TriggerStateRequest{Enabled: false, ExpectedVersion: 1}},
		{"POST", "/fire", api.FireTriggerRequest{RequestID: "other"}}, {"GET", "/occurrences", nil}, {"DELETE", "?expected_version=1", nil},
	} {
		response := triggerRequest(t, s, route.method, "/triggers/"+item.ID+route.suffix, other.ID, route.body)
		if response.Code != http.StatusNotFound {
			t.Fatalf("cross-client %s %s: %d %s", route.method, route.suffix, response.Code, response.Body.String())
		}
	}
	for _, path := range []string{"/triggers", "/triggers/occurrences"} {
		response := triggerRequest(t, s, "GET", path, other.ID, nil)
		if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "[]" {
			t.Fatalf("cross-client list = %s", response.Body.String())
		}
	}

	paused := triggerRequest(t, s, "PUT", "/triggers/"+item.ID+"/state", owner, api.TriggerStateRequest{Enabled: false, ExpectedVersion: 1})
	if paused.Code != http.StatusOK {
		t.Fatalf("pause: %s", paused.Body.String())
	}
	item = trigger.Trigger{}
	if err := json.Unmarshal(paused.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.Enabled || item.NextFireAt != nil || item.Version != 2 {
		t.Fatalf("paused = %#v", item)
	}
	stale := triggerRequest(t, s, "PUT", "/triggers/"+item.ID+"/state", owner, api.TriggerStateRequest{Enabled: true, ExpectedVersion: 1})
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale update = %d", stale.Code)
	}

	var first trigger.Occurrence
	for range 2 {
		response := triggerRequest(t, s, "POST", "/triggers/"+item.ID+"/fire", owner, api.FireTriggerRequest{RequestID: "click-1"})
		if response.Code != http.StatusAccepted {
			t.Fatalf("fire: %d %s", response.Code, response.Body.String())
		}
		var o trigger.Occurrence
		if err := json.Unmarshal(response.Body.Bytes(), &o); err != nil {
			t.Fatal(err)
		}
		if o.Status != "queued" || o.SessionID == "" || (first.ID != "" && first.ID != o.ID) {
			t.Fatalf("fire = %#v", o)
		}
		first = o
	}
	busy := triggerRequest(t, s, "POST", "/triggers/"+item.ID+"/fire", owner, api.FireTriggerRequest{RequestID: "click-2"})
	if !strings.Contains(busy.Body.String(), `"status":"skipped"`) {
		t.Fatalf("overlap = %s", busy.Body.String())
	}
	config := testTriggerDefinition()
	config.Source.Expression = "invalid"
	invalid := triggerRequest(t, s, "POST", "/triggers", owner, config)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid cron = %d", invalid.Code)
	}
	preview := triggerRequest(t, s, "POST", "/triggers/preview", owner, testTriggerDefinition().Source)
	var times api.TriggerPreview
	if err := json.Unmarshal(preview.Body.Bytes(), &times); err != nil || preview.Code != http.StatusOK || len(times.Times) != 5 {
		t.Fatalf("preview: %s, %v", preview.Body.String(), err)
	}
	deleted := triggerRequest(t, s, "DELETE", fmt.Sprintf("/triggers/%s?expected_version=%d", item.ID, item.Version), owner, nil)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete: %s", deleted.Body.String())
	}
	if _, err := data.GetSession(first.SessionID); err != nil {
		t.Fatalf("session lost after delete: %v", err)
	}
}

func TestTriggerSchedulerDueDowntimeAndFailures(t *testing.T) {
	for _, test := range []struct {
		name         string
		age          time.Duration
		startup      bool
		missingAgent bool
		want         string
	}{
		{"on time", time.Second, false, false, "queued"},
		{"downtime", time.Second, true, false, "skipped"},
		{"missed", 2 * time.Hour, false, false, "skipped"},
		{"missing agent", time.Second, false, true, "failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, data, owner := triggerTestServer(t, Config{})
			s.runs.stop()
			now := time.Date(2026, 9, 16, 8, 0, 1, 0, time.UTC)
			due := now.Add(-test.age)
			config := testTriggerDefinition()
			config.Source = trigger.Source{Type: "cron", Expression: "* * * * *", TimeZone: "UTC"}
			if test.missingAgent {
				config.Target.AgentID = "missing"
			}
			item, err := data.SaveTrigger(t.Context(), trigger.Trigger{Config: config, ClientID: owner, NextFireAt: &due}, 0)
			if err != nil {
				t.Fatal(err)
			}
			started := due.Add(-time.Hour)
			if test.startup {
				started = now
			}
			for range 2 {
				if err := s.triggers.tick(t.Context(), now, started); err != nil {
					t.Fatal(err)
				}
			}
			items, err := data.ListTriggerOccurrences(t.Context(), owner, item.ID, 50)
			if err != nil || len(items) != 1 || items[0].Status != test.want {
				t.Fatalf("occurrences = %#v, %v", items, err)
			}
			updated, err := data.GetTrigger(t.Context(), owner, item.ID)
			if err != nil || !updated.NextFireAt.After(now) {
				t.Fatalf("next time = %#v, %v", updated, err)
			}
			sessions, err := data.ListSessions()
			if err != nil {
				t.Fatal(err)
			}
			if test.want != "queued" && len(sessions) != 0 {
				t.Fatalf("unexpected sessions: %d", len(sessions))
			}
		})
	}
}

func TestTriggerRunUsesNormalExecutionAndHistory(t *testing.T) {
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Review complete.\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"); err != nil {
			t.Error(err)
		}
	}))
	defer upstream.Close()
	s, data, owner := triggerTestServer(t, Config{})
	agent, err := data.GetAgent("agt_trigger")
	if err != nil {
		t.Fatal(err)
	}
	agent.Options[agentOptionModelRoute] = models.ModelInfo{Provider: "test", ID: "model", API: models.APIOpenAICompatible, BaseURL: upstream.URL}
	if err := data.UpdateAgent(agent); err != nil {
		t.Fatal(err)
	}
	due := time.Now().UTC().Truncate(time.Minute)
	item, err := data.SaveTrigger(t.Context(), trigger.Trigger{Config: testTriggerDefinition(), ClientID: owner, NextFireAt: &due}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.triggers.tick(t.Context(), due.Add(time.Second), due.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for {
		items, err := data.ListTriggerOccurrences(t.Context(), owner, item.ID, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) == 1 && items[0].Status != "queued" && items[0].Status != "running" {
			if items[0].Status != "completed" {
				t.Fatalf("run = %#v", items[0])
			}
			messages, err := data.ListMessages(t.Context(), items[0].SessionID)
			if err != nil || len(messages) < 2 {
				t.Fatalf("messages = %#v, %v", messages, err)
			}
			if requests.Load() != 1 {
				t.Fatalf("provider calls = %d", requests.Load())
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("run did not finish")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestTriggerAPIRootThroughGoConsoleWorkflows(t *testing.T) {
	for _, mode := range []string{"embedded", "proxied"} {
		t.Run(mode, func(t *testing.T) {
			var consoleRequests atomic.Int32
			vite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				consoleRequests.Add(1)
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprint(w, "<html>Console development</html>")
			}))
			defer vite.Close()
			cfg := Config{Password: "test-password"}
			if mode == "proxied" {
				cfg.ConsoleDevURL = vite.URL
			}
			s, _, owner := triggerTestServer(t, cfg)
			unauthorized := triggerRequest(t, s, "GET", "/triggers", owner, nil)
			if unauthorized.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated = %d", unauthorized.Code)
			}
			for _, path := range []string{"/triggers", "/triggers/occurrences", "/console/triggers"} {
				r := httptest.NewRequest("GET", path, nil)
				r.SetBasicAuth("wingman", "test-password")
				w := httptest.NewRecorder()
				s.ServeHTTP(w, r)
				if w.Code != http.StatusOK {
					t.Fatalf("%s = %d: %s", path, w.Code, w.Body.String())
				}
				if strings.HasPrefix(path, "/triggers") && strings.TrimSpace(w.Body.String()) != "[]" {
					t.Fatalf("API reached Console: %s", w.Body.String())
				}
			}
			if mode == "proxied" && consoleRequests.Load() != 1 {
				t.Fatalf("Vite received %d requests", consoleRequests.Load())
			}
		})
	}
}

func TestTriggerManagerStartsAndStopsWithServer(t *testing.T) {
	s, data, owner := triggerTestServer(t, Config{})
	due := time.Now().UTC().Add(-time.Hour).Truncate(time.Minute)
	item, err := data.SaveTrigger(t.Context(), trigger.Trigger{Config: testTriggerDefinition(), ClientID: owner, NextFireAt: &due}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for {
		items, err := data.ListTriggerOccurrences(t.Context(), owner, item.ID, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) == 1 {
			if items[0].Status != "skipped" {
				t.Fatalf("startup dispatched stale work: %#v", items[0])
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("scheduler did not start")
		case <-time.After(time.Millisecond):
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.triggers.done:
	default:
		t.Fatal("scheduler did not stop")
	}
}
