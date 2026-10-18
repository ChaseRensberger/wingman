package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
)

func TestCompletionRequestPathsAndRestart(t *testing.T) {
	for _, workflow := range []string{"production", "proxied-development", "vite-development"} {
		t.Run(workflow, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
			defer cancel()
			data, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "completion.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer data.Close()
			owner, err := data.EnsureDefaultClient()
			if err != nil {
				t.Fatal(err)
			}
			other, err := data.CreateClient("other")
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer upstream.Close()
			agent := store.Agent{ID: "agt_completion", ModelRef: "test/model", Options: map[string]any{agentOptionModelRoute: models.ModelInfo{Provider: "test", ID: "model", API: models.APIOpenAICompatible, BaseURL: upstream.URL}}}
			if err := data.CreateAgent(&agent); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"ses_completion", "ses_other"} {
				if err := data.CreateSession(&store.Session{ID: id, ClientID: owner.ID}); err != nil {
					t.Fatal(err)
				}
			}
			frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Errorf("API reached Console frontend: %s", r.URL.Path) }))
			defer frontend.Close()
			cfg := Config{Store: data, MaxActiveRuns: 1, Username: "test", Password: "recovery-test-password"}
			if workflow == "proxied-development" {
				cfg.ConsoleDevURL = frontend.URL
			}
			server := New(cfg)
			if err := server.Start(ctx); err != nil {
				t.Fatal(err)
			}
			host := httptest.NewServer(server)
			defer host.Close()
			defer func() { _ = server.Close(context.Background()) }()
			target := host.URL
			if workflow == "vite-development" {
				target = recoveryViteProxy(t, host.URL)
			}
			request := func(method, path string, input any, clientID string, auth bool) (int, []byte) {
				t.Helper()
				body, _ := json.Marshal(input)
				req, err := http.NewRequestWithContext(ctx, method, target+path, bytes.NewReader(body))
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
				output, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				return response.StatusCode, output
			}
			admit := func(id string, remote bool) string {
				t.Helper()
				status, output := request("POST", "/sessions/"+id+"/message", api.MessageSessionRequest{AgentID: agent.ID, Message: "go", Completion: api.CompletionOptions{Remote: remote}}, "", true)
				var admitted api.MessageSessionResponse
				if err := json.Unmarshal(output, &admitted); err != nil || status != 202 {
					t.Fatalf("admit=%d %s %v", status, output, err)
				}
				return admitted.RunID
			}
			await := func(id, runID, want string) *store.SessionRun {
				t.Helper()
				for {
					current, err := data.GetSessionRun(ctx, id, runID)
					if err != nil {
						t.Fatal(err)
					}
					if current.Status == want {
						return current
					}
					if current.Status == "failed" {
						t.Fatalf("run failed: %#v", current)
					}
					select {
					case <-ctx.Done():
						t.Fatalf("await %s: got %s: %v", want, current.Status, ctx.Err())
					case <-time.After(time.Millisecond):
					}
				}
			}
			runID := admit("ses_completion", true)
			pending := await("ses_completion", runID, store.SessionRunStatusAwaitingCompletion)
			queuedID := admit("ses_completion", false)
			otherID := admit("ses_other", false)
			await("ses_other", otherID, store.SessionRunStatusCompleted)
			queued, _ := data.GetSessionRun(ctx, "ses_completion", queuedID)
			if queued.Status != store.SessionRunStatusQueued || calls.Load() != 2 {
				t.Fatalf("waiting lost session ordering or slot: queued=%s calls=%d", queued.Status, calls.Load())
			}
			path := "/sessions/ses_completion/runs/" + runID + "/completion"
			candidateID := pending.Completions[0].CandidateID
			reply := api.CompletionReplyRequest{CandidateID: candidateID, Decision: api.CompletionDecision{Action: "continue", Feedback: "fix tests"}}
			if workflow != "vite-development" {
				if status, _ := request("POST", path, reply, "", false); status != 401 {
					t.Fatalf("unauthenticated reply=%d", status)
				}
			}
			if status, output := request("POST", path, reply, other.ID, true); status != 403 {
				t.Fatalf("cross-client reply=%d %s", status, output)
			}
			stale := reply
			stale.CandidateID = "msg_stale"
			if status, _ := request("POST", path, stale, "", true); status != 409 {
				t.Fatalf("stale reply=%d", status)
			}
			// Reopen the daemon while validation is pending. A disconnect must not accept it.
			if err := server.Close(ctx); err != nil {
				t.Fatal(err)
			}
			host.Close()
			server = New(cfg)
			if err := server.Start(ctx); err != nil {
				t.Fatal(err)
			}
			host = httptest.NewServer(server)
			defer host.Close()
			target = host.URL
			if workflow == "vite-development" {
				target = recoveryViteProxy(t, host.URL)
			}
			await("ses_completion", runID, store.SessionRunStatusAwaitingCompletion)
			for range 2 {
				if status, output := request("POST", path, reply, "", true); status != 200 {
					t.Fatalf("reply=%d %s", status, output)
				}
			}
			for {
				pending = await("ses_completion", runID, store.SessionRunStatusAwaitingCompletion)
				if len(pending.Completions) == 2 {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(time.Millisecond):
				}
			}
			if pending.RecoveryAttempts != 0 || calls.Load() != 3 {
				t.Fatalf("resumed=%#v calls=%d", pending, calls.Load())
			}
			accept := api.CompletionReplyRequest{CandidateID: pending.Completions[1].CandidateID, Decision: api.CompletionDecision{Action: "accept"}}
			if status, output := request("POST", path, accept, "", true); status != 200 {
				t.Fatalf("accept=%d %s", status, output)
			}
			await("ses_completion", runID, store.SessionRunStatusCompleted)
			await("ses_completion", queuedID, store.SessionRunStatusCompleted)
			if calls.Load() != 4 {
				t.Fatalf("accept dispatched another provider request: %d", calls.Load())
			}
			status, output := request("GET", "/sessions/ses_completion/events/history", nil, "", true)
			if status != 200 {
				t.Fatalf("history=%d %s", status, output)
			}
			if !bytes.Contains(output, []byte("session.run.completion")) {
				t.Fatalf("missing replayable completion events: %s", output)
			}
			// Cancellation while suspended frees the session and rejects late acceptance.
			cancelID := admit("ses_completion", true)
			pending = await("ses_completion", cancelID, store.SessionRunStatusAwaitingCompletion)
			if status, output := request("POST", "/sessions/ses_completion/abort", nil, "", true); status != 200 {
				t.Fatalf("abort=%d %s", status, output)
			}
			await("ses_completion", cancelID, store.SessionRunStatusAborted)
			accept.CandidateID = pending.Completions[0].CandidateID
			if status, output := request("POST", "/sessions/ses_completion/runs/"+cancelID+"/completion", accept, "", true); status != 409 {
				t.Fatalf("late acceptance=%d %s", status, output)
			}
		})
	}
}
