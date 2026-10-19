package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/plugins/orientation"
	"github.com/chaserensberger/wingman/skill"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestStartupContextRecordedForExplicitCodingAgents(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()
	work := t.TempDir()
	if out, err := exec.Command("git", "-C", work, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	canonical, err := filepath.EvalSymlinks(work)
	if err != nil {
		t.Fatal(err)
	}
	guide := filepath.Join(work, "AGENTS.md")
	if err := os.WriteFile(guide, []byte("Original project instructions"), 0644); err != nil {
		t.Fatal(err)
	}
	data := memory.NewStore()
	s := New(Config{Store: data, References: []orientation.Reference{{Name: "manual", Path: work, Description: "Reference manuals"}}})
	defer s.Close(t.Context())
	agents := store.DefaultAgents()
	agents[0].Name = "Renamed builder"
	agents = append(agents, &store.Agent{Name: "Build"}, &store.Agent{Name: "Custom", Plugins: []string{"wingcode"}})
	for i, agent := range agents {
		t.Run(agent.Name, func(t *testing.T) {
			agent.ModelRef = "test/model"
			agent.Options = map[string]any{agentOptionModelRoute: models.ModelInfo{Provider: "test", ID: "model", API: models.APIOpenAICompatible, BaseURL: upstream.URL, Capabilities: models.ModelCapabilities{Tools: true}}}
			agent.Permissions = permission.Ruleset{{Action: "edit", Resource: "*", Effect: permission.EffectDeny}}
			sess := &store.Session{ID: fmt.Sprintf("ses_startup_%d", i), WorkDir: work}
			if err := data.CreateSession(sess); err != nil {
				t.Fatal(err)
			}
			built, err := s.buildSessionWithSkills(t.Context(), agent, sess, []skill.Skill{{ID: "manual", Name: "Manual lookup", Description: "Read reference manuals"}})
			if err != nil {
				t.Fatal(err)
			}
			defer built.Close(t.Context())
			if _, err := built.Run(t.Context(), "hello"); err != nil {
				t.Fatal(err)
			}
			calls, err := data.ListModelCalls(t.Context(), sess.ID)
			if err != nil || len(calls) != 1 {
				t.Fatalf("calls=%v err=%v", calls, err)
			}
			input, err := data.GetModelCallInput(t.Context(), sess.ID, calls[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, text := range []string{"Current date:", "Working directory: " + canonical, "Platform:", "Temporary directory:", "Provider ID: test", "Model ID: model", "manual (Manual lookup)", "Reference manuals", "Original project instructions"} {
				if !strings.Contains(input.System, text) {
					t.Fatalf("missing %q in %s", text, input.System)
				}
			}
			coding := len(agent.Plugins) > 0
			if strings.Contains(input.System, "## Coding workflow") != coding || strings.Contains(input.System, "Repository root: "+canonical) != coding {
				t.Fatalf("coding context selected by name rather than plugins: %s", input.System)
			}
			for _, def := range input.Tools {
				if def.Name == "write" || def.Name == "edit" || def.Name == "apply_patch" {
					t.Fatalf("denied tool advertised: %s", def.Name)
				}
			}
			if i == 0 {
				if err := os.WriteFile(guide, []byte("Changed project instructions"), 0644); err != nil {
					t.Fatal(err)
				}
				if _, err := built.Run(t.Context(), "again"); err != nil {
					t.Fatal(err)
				}
				old, err := data.GetModelCallInput(t.Context(), sess.ID, calls[0].ID)
				if err != nil || old.System != input.System {
					t.Fatal("historical input changed")
				}
				calls, err = data.ListModelCalls(t.Context(), sess.ID)
				if err != nil || len(calls) != 2 {
					t.Fatalf("calls=%v err=%v", calls, err)
				}
				found := false
				for _, call := range calls {
					current, err := data.GetModelCallInput(t.Context(), sess.ID, call.ID)
					if err != nil {
						t.Fatal(err)
					}
					found = found || strings.Contains(current.System, "Changed project instructions")
				}
				if !found {
					t.Fatal("next call did not refresh instructions")
				}
				if err := os.WriteFile(guide, []byte("Original project instructions"), 0644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestAgentPluginRequestPaths(t *testing.T) {
	frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Errorf("API reached frontend: %s", r.URL.Path) }))
	defer frontend.Close()
	for _, workflow := range []string{"production", "proxied-development", "vite-development"} {
		t.Run(workflow, func(t *testing.T) {
			cfg := Config{Store: memory.NewStore(), Username: "test", Password: "recovery-test-password"}
			if workflow == "proxied-development" {
				cfg.ConsoleDevURL = frontend.URL
			}
			s := New(cfg)
			defer s.Close(context.Background())
			host := httptest.NewServer(s)
			defer host.Close()
			target := host.URL
			if workflow == "vite-development" {
				target = recoveryViteProxy(t, host.URL)
			}
			request := func(method, path, body string, want int) api.Agent {
				t.Helper()
				req, err := http.NewRequestWithContext(t.Context(), method, target+path, strings.NewReader(body))
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
					data, _ := io.ReadAll(response.Body)
					t.Fatalf("%s %s: %d %s", method, path, response.StatusCode, data)
				}
				var agent api.Agent
				if want < 300 {
					if err := json.NewDecoder(response.Body).Decode(&agent); err != nil {
						t.Fatal(err)
					}
				}
				return agent
			}
			for _, body := range []string{`{"name":"bad","plugins":["missing"]}`, `{"name":"bad","plugins":["wingcode","wingcode"]}`} {
				request("POST", "/agents", body, 400)
			}
			a := request("POST", "/agents", `{"name":"custom","plugins":["wingcode"]}`, 201)
			got := request("GET", "/agents/"+a.ID, "", 200)
			if len(got.Plugins) != 1 || got.Plugins[0] != "wingcode" {
				t.Fatalf("plugins=%v", got.Plugins)
			}
			got = request("PUT", "/agents/"+a.ID, `{"name":"renamed"}`, 200)
			if len(got.Plugins) != 1 {
				t.Fatal("rename removed plugins")
			}
			request("PUT", "/agents/"+a.ID, `{"plugins":["missing"]}`, 400)
			got = request("PUT", "/agents/"+a.ID, `{"plugins":[]}`, 200)
			if len(got.Plugins) != 0 {
				t.Fatal("plugins not cleared")
			}
		})
	}
}
