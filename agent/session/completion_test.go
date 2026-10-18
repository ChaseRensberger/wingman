package session

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
)

func completionOptions(data store.Store, id string, client models.Client) []Option {
	return []Option{WithID("ses_history"), WithRunID(id), WithStore(data), WithClient(client), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{})}
}

type completionClient struct {
	requestCaptureClient
	requests []models.Request
}

func (c *completionClient) Stream(ctx context.Context, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	c.requests = append(c.requests, req)
	return c.requestCaptureClient.Stream(ctx, req)
}

func TestRemoteCompletionResumeAndLimit(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			data := historyStore(t, backend)
			options := store.CompletionOptions{Remote: true, MaxContinuations: 1}
			admitted, err := data.AdmitSessionRun(t.Context(), store.SessionRun{SessionID: "ses_history", Message: "go", Completion: options})
			if err != nil {
				t.Fatal(err)
			}
			client := &completionClient{}
			base := append(completionOptions(data, admitted.Run.ID, client), WithCompletionOptions(options))
			for iteration := range 2 {
				claimed, err := data.ClaimNextSessionRun(t.Context(), "ses_history")
				if err != nil || !claimed.Changed {
					t.Fatalf("claim=%#v err=%v", claimed, err)
				}
				opts := append([]Option(nil), base...)
				if iteration > 0 {
					opts = append(opts, WithRunRecovery())
				}
				s := New(opts...)
				_, err = s.Run(t.Context(), "go")
				_ = s.Close(context.Background())
				if !errors.Is(err, run.ErrCompletionPending) {
					t.Fatal(err)
				}
				current, err := data.GetSessionRun(t.Context(), "ses_history", admitted.Run.ID)
				if err != nil || current.Status != store.SessionRunStatusAwaitingCompletion {
					t.Fatalf("run=%#v err=%v", current, err)
				}
				c := current.Completions[len(current.Completions)-1]
				c.Status, c.Feedback = "continue", "fix it"
				update := store.CompletionUpdate{SessionID: "ses_history", RunID: current.ID, ExpectedStatus: "waiting", Completion: c}
				if _, err := data.UpdateSessionRunCompletion(t.Context(), update); err != nil {
					t.Fatal(err)
				}
				duplicate, err := data.UpdateSessionRunCompletion(t.Context(), update)
				if err != nil || duplicate.Changed {
					t.Fatalf("duplicate=%#v err=%v", duplicate, err)
				}
			}
			if _, err := data.ClaimNextSessionRun(t.Context(), "ses_history"); err != nil {
				t.Fatal(err)
			}
			s := New(append(base, WithRunRecovery())...)
			defer s.Close(context.Background())
			if _, err := s.Run(t.Context(), "go"); !errors.Is(err, run.ErrCompletionLimit) {
				t.Fatalf("limit=%v", err)
			}
			messages, err := data.ListMessages(t.Context(), "ses_history")
			if err != nil {
				t.Fatal(err)
			}
			roles := []string{}
			for _, m := range messages {
				roles = append(roles, m.Role)
			}
			if !reflect.DeepEqual(roles, []string{"user", "assistant", "user", "assistant"}) {
				t.Fatalf("roles=%v", roles)
			}
			if len(client.requests) != 2 {
				t.Fatalf("provider calls=%d", len(client.requests))
			}
		})
	}
}

func TestRemoteStructuredOutputWaitsForAcceptance(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			data := historyStore(t, backend)
			admitted, err := data.AdmitSessionRun(t.Context(), store.SessionRun{SessionID: "ses_history", Message: "go", Completion: store.CompletionOptions{Remote: true}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := data.ClaimNextSessionRun(t.Context(), "ses_history"); err != nil {
				t.Fatal(err)
			}
			client := &sequencedToolClient{messages: []models.Message{{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: `{"answer":"accepted"}`}}}}}
			options := append(completionOptions(data, admitted.Run.ID, client),
				WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{Capabilities: models.ModelCapabilities{StructuredOutput: true}}),
				WithOutputSchema(&models.OutputSchema{Name: "answer", Schema: map[string]any{"type": "object"}}),
			)
			execute := func(opts []Option) (*Result, int, error) {
				t.Helper()
				sess := New(opts...)
				defer sess.Close(context.Background())
				stream, err := sess.RunStream(t.Context(), "go")
				if err != nil {
					t.Fatal(err)
				}
				outputs := 0
				for stream.Next() {
					if stream.Event().Type == "structured_output" {
						outputs++
					}
				}
				return stream.Result(), outputs, stream.Err()
			}
			result, outputs, err := execute(options)
			if !errors.Is(err, run.ErrCompletionPending) || outputs != 0 || result.StructuredOutput != nil {
				t.Fatalf("pending outputs=%d result=%#v err=%v", outputs, result, err)
			}
			calls, err := data.QueryModelCalls(t.Context(), "ses_history", store.ModelCallQuery{RunID: admitted.Run.ID})
			if err != nil || len(calls) != 1 || len(calls[0].StructuredOutputJSON) != 0 {
				t.Fatalf("pending calls=%#v err=%v", calls, err)
			}
			current, err := data.GetSessionRun(t.Context(), "ses_history", admitted.Run.ID)
			if err != nil {
				t.Fatal(err)
			}
			candidate := current.Completions[0]
			candidate.Status = "accept"
			if _, err := data.UpdateSessionRunCompletion(t.Context(), store.CompletionUpdate{SessionID: "ses_history", RunID: current.ID, ExpectedStatus: "waiting", Completion: candidate}); err != nil {
				t.Fatal(err)
			}
			if _, err := data.ClaimNextSessionRun(t.Context(), "ses_history"); err != nil {
				t.Fatal(err)
			}
			result, outputs, err = execute(append(options, WithRunRecovery()))
			if err != nil || outputs != 1 || result.StructuredOutput["answer"] != "accepted" || client.next != 1 {
				t.Fatalf("accepted outputs=%d result=%#v calls=%d err=%v", outputs, result, client.next, err)
			}
		})
	}
}

type completionTestPlugin struct {
	hook run.BeforeCompleteHook
}

func (completionTestPlugin) Name() string { return "completion-test" }

func (p completionTestPlugin) Activate(registry *plugin.Registry) (plugin.Cleanup, error) {
	return nil, registry.RegisterBeforeComplete(p.hook)
}

func TestPluginsPassBeforeRemoteCompletion(t *testing.T) {
	data := historyStore(t, "sqlite")
	admitted, err := data.AdmitSessionRun(t.Context(), store.SessionRun{SessionID: "ses_history", Message: "go", Completion: store.CompletionOptions{Remote: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.ClaimNextSessionRun(t.Context(), "ses_history"); err != nil {
		t.Fatal(err)
	}
	var seen []string
	client := &completionClient{}
	opts := append(completionOptions(data, admitted.Run.ID, client),
		WithPlugin(completionTestPlugin{hook: func(_ context.Context, info run.CompletionInfo) (run.CompletionDecision, error) {
			seen = append(seen, "plugin")
			if info.Step == 1 {
				return run.CompletionDecision{Action: "continue", Feedback: "fix"}, nil
			}
			return run.CompletionDecision{Action: "accept"}, nil
		}}),
		WithBeforeComplete(func(context.Context, run.CompletionInfo) (run.CompletionDecision, error) {
			seen = append(seen, "application")
			return run.CompletionDecision{Action: "accept"}, nil
		}),
	)
	sess := New(opts...)
	defer sess.Close(context.Background())
	if _, err := sess.Run(t.Context(), "go"); !errors.Is(err, run.ErrCompletionPending) {
		t.Fatalf("run: %v", err)
	}
	if !reflect.DeepEqual(seen, []string{"plugin", "plugin", "application"}) {
		t.Fatalf("validators=%v", seen)
	}
	current, err := data.GetSessionRun(t.Context(), "ses_history", admitted.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != store.SessionRunStatusAwaitingCompletion || len(current.Completions) != 2 || current.Completions[0].Status != "continue" || current.Completions[1].Status != "waiting" || len(client.requests) != 2 {
		t.Fatalf("run=%#v requests=%d", current, len(client.requests))
	}
}

type completionCrashStore struct {
	store.Store
	stage string
}

func (s *completionCrashStore) UpdateSessionRunCompletion(ctx context.Context, update store.CompletionUpdate) (store.SessionRunTransition, error) {
	transition, err := s.Store.UpdateSessionRunCompletion(ctx, update)
	if err == nil && update.Completion.Status == s.stage {
		os.Exit(23)
	}
	return transition, err
}

func (s *completionCrashStore) SaveMessage(ctx context.Context, message store.StoredMessage) error {
	err := s.Store.SaveMessage(ctx, message)
	if err == nil && s.stage == "feedback" && strings.Contains(string(message.MetadataJSON), "completion_validator") {
		os.Exit(23)
	}
	return err
}

func TestCompletionRecoveryAcrossProcessCrashes(t *testing.T) {
	if stage := os.Getenv("WINGMAN_COMPLETION_CRASH_STAGE"); stage != "" {
		data, err := store.NewSQLiteStore(os.Getenv("WINGMAN_COMPLETION_CRASH_DB"))
		if err != nil {
			t.Fatal(err)
		}
		defer data.Close()
		opts := completionOptions(&completionCrashStore{Store: data, stage: stage}, os.Getenv("WINGMAN_COMPLETION_CRASH_RUN"), &requestCaptureClient{})
		opts = append(opts, WithBeforeComplete(func(context.Context, run.CompletionInfo) (run.CompletionDecision, error) {
			action := "continue"
			feedback := "fix"
			if stage == "accept" {
				action, feedback = "accept", ""
			}
			return run.CompletionDecision{Action: action, Feedback: feedback}, nil
		}))
		_, err = New(opts...).Run(t.Context(), "go")
		t.Fatalf("did not crash: %v", err)
	}
	for _, stage := range []string{"validating", "continue", "feedback", "accept"} {
		t.Run(stage, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "completion.db")
			data, err := store.NewSQLiteStore(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := data.CreateSession(&store.Session{ID: "ses_history"}); err != nil {
				t.Fatal(err)
			}
			admitted, err := data.AdmitSessionRun(t.Context(), store.SessionRun{SessionID: "ses_history", Message: "go"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := data.ClaimNextSessionRun(t.Context(), "ses_history"); err != nil {
				t.Fatal(err)
			}
			if err := data.Close(); err != nil {
				t.Fatal(err)
			}
			command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestCompletionRecoveryAcrossProcessCrashes$")
			command.Env = append(os.Environ(), "WINGMAN_COMPLETION_CRASH_STAGE="+stage, "WINGMAN_COMPLETION_CRASH_DB="+path, "WINGMAN_COMPLETION_CRASH_RUN="+admitted.Run.ID)
			output, err := command.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 23 {
				t.Fatalf("crash: %v\n%s", err, output)
			}
			data, err = store.NewSQLiteStore(path)
			if err != nil {
				t.Fatal(err)
			}
			defer data.Close()
			if _, err := data.RequeueSessionRun(t.Context(), admitted.Run.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := data.ClaimNextSessionRun(t.Context(), "ses_history"); err != nil {
				t.Fatal(err)
			}
			client := &completionClient{}
			hookCalls := 0
			opts := append(completionOptions(data, admitted.Run.ID, client), WithRunRecovery(), WithBeforeComplete(func(_ context.Context, info run.CompletionInfo) (run.CompletionDecision, error) {
				hookCalls++
				if info.Step == 1 {
					if stage != "validating" {
						t.Error("repeated committed validation")
					}
					return run.CompletionDecision{Action: "continue", Feedback: "fix"}, nil
				}
				return run.CompletionDecision{Action: "accept"}, nil
			}))
			sess := New(opts...)
			defer sess.Close(context.Background())
			if _, err := sess.Run(t.Context(), "go"); err != nil {
				t.Fatal(err)
			}
			wantRequests, wantHooks := 1, 1
			if stage == "validating" {
				wantHooks++
			}
			if stage == "accept" {
				wantRequests, wantHooks = 0, 0
			}
			if len(client.requests) != wantRequests || hookCalls != wantHooks {
				t.Fatalf("requests=%d hooks=%d", len(client.requests), hookCalls)
			}
			messages, err := data.ListMessages(t.Context(), "ses_history")
			if err != nil {
				t.Fatal(err)
			}
			wantRoles := []string{"user", "assistant", "user", "assistant"}
			if stage == "accept" {
				wantRoles = wantRoles[:2]
			}
			roles := []string{}
			for _, m := range messages {
				roles = append(roles, m.Role)
			}
			if !reflect.DeepEqual(roles, wantRoles) {
				t.Fatalf("roles=%v", roles)
			}
			if len(client.requests) > 0 {
				request := client.requests[0]
				if len(request.Messages) != 3 || request.Messages[1].Role != models.RoleAssistant || request.Messages[2].Role != models.RoleUser {
					t.Fatalf("replayed context=%#v", request.Messages)
				}
			}
		})
	}
}
