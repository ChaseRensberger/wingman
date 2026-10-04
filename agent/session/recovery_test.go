package session

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/plugins/compaction"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/tool"
)

func TestRunRecoveryAcrossProcessCrashes(t *testing.T) {
	for _, test := range []struct {
		stage      string
		safe       bool
		blocked    bool
		modelCalls int
		toolCalls  int
	}{
		{"input", true, false, 2, 1},
		{"checkpoint", true, false, 2, 1},
		{"dispatch", true, false, 3, 1},
		{"partial", true, false, 3, 1},
		{"retry_scheduled", true, false, 3, 1},
		{"retry_terminal", true, true, 1, 0},
		{"reset", true, true, 1, 1},
		{"model", true, false, 2, 1},
		{"proposed", true, false, 2, 1},
		{"authorized", false, false, 2, 1},
		{"started", true, false, 2, 1},
		{"effect", true, false, 2, 2},
		{"batch", true, false, 2, 2},
		{"completed", false, false, 2, 1},
		{"final", false, false, 2, 1},
		{"started", false, true, 1, 0},
		{"effect", false, true, 1, 1},
	} {
		name := test.stage
		if !test.safe {
			name += "-unsafe"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			safe := "false"
			if test.safe {
				safe = "true"
			}
			stages := []string{test.stage}
			if test.stage == "reset" {
				stages = []string{"effect", "reset"}
			}
			for _, stage := range stages {
				command := exec.Command(os.Args[0], "-test.run=^TestRunRecoveryCrashHelper$")
				command.Env = append(os.Environ(), "WINGMAN_RECOVERY_DIR="+dir, "WINGMAN_RECOVERY_STAGE="+stage, "WINGMAN_RECOVERY_SAFE="+safe)
				output, err := command.CombinedOutput()
				var exited *exec.ExitError
				if !errors.As(err, &exited) || exited.ExitCode() != 23 {
					t.Fatalf("crash helper: %v\n%s", err, output)
				}
			}
			data, err := store.NewSQLiteStore(filepath.Join(dir, "wingman.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer data.Close()
			ctx := context.Background()
			if err := data.InterruptActiveToolUses(ctx); err != nil {
				t.Fatal(err)
			}
			if err := data.InterruptActiveModelCalls(ctx, "run_recover", "process_interrupted", "process stopped"); err != nil {
				t.Fatal(err)
			}
			if err := RecoverRunMessages(ctx, data, "ses_recover", "run_recover"); err != nil {
				t.Fatal(err)
			}
			if _, err := data.RequeueSessionRun(ctx, "run_recover"); err != nil {
				t.Fatal(err)
			}
			claimed, err := data.ClaimNextSessionRun(ctx, "ses_recover")
			if err != nil || claimed.Run.ID != "run_recover" || claimed.Run.RecoveryAttempts != len(stages) {
				t.Fatalf("claim = %#v, %v", claimed, err)
			}
			sess := recoveryTestSession(data, dir, "", test.safe && test.stage != "reset", true)
			result, err := sess.Run(ctx, "inspect")
			if test.blocked {
				if !errors.Is(err, ErrRecoveryBlocked) {
					t.Fatalf("recovery = %#v, %v", result, err)
				}
			} else {
				if err != nil || result.Response != "done" {
					t.Fatalf("recovery = %#v, %v", result, err)
				}
				uses, err := data.ListToolUses(ctx, "ses_recover")
				wantUses := 1
				if test.stage == "batch" {
					wantUses = 2
				}
				if err != nil || len(uses) != wantUses || uses[wantUses-1].Status != store.ToolUseStatusCompleted || uses[wantUses-1].Output != "read result" {
					t.Fatalf("tool records = %#v, %v", uses, err)
				}
				if !strings.Contains(string(uses[wantUses-1].StructuredJSON), "retained") || !strings.Contains(string(uses[wantUses-1].OutputPartsJSON), "extra result") {
					t.Fatalf("lost structured or multipart result: %#v", uses[wantUses-1])
				}
			}
			permissions, writes := 1, 0
			if test.stage == "retry_terminal" {
				permissions = 0
			}
			if test.stage == "batch" {
				permissions, writes = 2, 1
			}
			for file, want := range map[string]int{"models": test.modelCalls, "tools": test.toolCalls, "permissions": permissions, "writes": writes} {
				content, err := os.ReadFile(filepath.Join(dir, file))
				if err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				if len(content) != want {
					t.Fatalf("%s executions = %d, want %d", file, len(content), want)
				}
			}
			messages, err := data.ListMessages(ctx, "ses_recover")
			if err != nil {
				t.Fatal(err)
			}
			users := 0
			for _, message := range messages {
				if message.Role == "user" {
					users++
				}
			}
			if users != 1 {
				t.Fatalf("input submitted %d times", users)
			}
			if test.stage == "retry_scheduled" {
				calls, err := data.ListModelCalls(ctx, "ses_recover")
				if err != nil || len(calls) != 3 || calls[0].AssistantMessageID == "" || calls[1].Step != 1 || calls[1].Attempt != 2 {
					t.Fatalf("retry attempt records = %#v, %v", calls, err)
				}
			}
			if test.stage == "partial" {
				found := false
				for _, message := range messages {
					if message.State == "failed" && len(message.Parts) > 0 && strings.Contains(string(message.Parts[0].PayloadJSON), "partial answer") {
						found = true
					}
				}
				if !found {
					t.Fatal("partial answer was discarded")
				}
			}
			events, err := data.ListAggregateEvents(ctx, store.AggregateRef{Type: store.AggregateSession, ID: "ses_recover"}, 0, 1000)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.ProjectSessionRuns(events); err != nil {
				t.Fatal(err)
			}
			if _, err := store.ProjectSessionToolUses(events); err != nil {
				t.Fatal(err)
			}
			if err := data.RebuildSessionProjections(ctx, "ses_recover"); err != nil {
				t.Fatal(err)
			}
			rebuilt, err := data.GetSessionRun(ctx, "ses_recover", "run_recover")
			if err != nil || rebuilt.RecoveryAttempts != len(stages) {
				t.Fatalf("rebuilt = %#v, %v", rebuilt, err)
			}
		})
	}
}

func TestRunRecoveryCrashHelper(t *testing.T) {
	dir := os.Getenv("WINGMAN_RECOVERY_DIR")
	if dir == "" {
		return
	}
	data, err := store.NewSQLiteStore(filepath.Join(dir, "wingman.db"))
	if err != nil {
		t.Fatal(err)
	}
	stage := os.Getenv("WINGMAN_RECOVERY_STAGE")
	ctx := context.Background()
	if stage == "reset" {
		if err := data.InterruptActiveToolUses(ctx); err != nil {
			t.Fatal(err)
		}
		if err := RecoverRunMessages(ctx, data, "ses_recover", "run_recover"); err != nil {
			t.Fatal(err)
		}
		if _, err := data.RequeueSessionRun(ctx, "run_recover"); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := data.CreateSession(&store.Session{ID: "ses_recover"}); err != nil {
			t.Fatal(err)
		}
		if _, err := data.AdmitSessionRun(ctx, store.SessionRun{ID: "run_recover", SessionID: "ses_recover", Message: "inspect"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := data.ClaimNextSessionRun(ctx, "ses_recover"); err != nil {
		t.Fatal(err)
	}
	sess := recoveryTestSession(&recoveryCrashStore{Store: data, stage: stage}, dir, stage, os.Getenv("WINGMAN_RECOVERY_SAFE") == "true", stage == "reset")
	if stage == "retry_terminal" {
		WithRetryPolicy(run.RetryPolicy{MaxAttempts: 1})(sess)
	}
	_, err = sess.Run(ctx, "inspect")
	t.Fatalf("did not crash: %v", err)
}

type recoveryCrashStore struct {
	store.Store
	stage string
}

func (s *recoveryCrashStore) SaveMessage(ctx context.Context, message store.StoredMessage) error {
	if err := s.Store.SaveMessage(ctx, message); err != nil {
		return err
	}
	if s.stage == "input" && message.Role == "user" {
		os.Exit(23)
	}
	if s.stage == "checkpoint" && message.Role == "assistant" && len(message.Parts) == 0 {
		os.Exit(23)
	}
	if s.stage == "partial" && len(message.Parts) > 0 && strings.Contains(string(message.Parts[0].PayloadJSON), "partial answer") {
		os.Exit(23)
	}
	if s.stage == "final" && message.Role == "assistant" && message.State == "completed" && len(message.Parts) == 1 && message.Parts[0].Kind == "text" {
		os.Exit(23)
	}
	return nil
}

func (s *recoveryCrashStore) UpsertModelCall(ctx context.Context, call store.ModelCall) error {
	if err := s.Store.UpsertModelCall(ctx, call); err != nil {
		return err
	}
	if s.stage == "model" && call.Status == store.ModelCallStatusCompleted {
		os.Exit(23)
	}
	if strings.HasPrefix(s.stage, "retry_") && call.Status == store.ModelCallStatusFailed {
		os.Exit(23)
	}
	return nil
}

func (s *recoveryCrashStore) SaveToolUse(ctx context.Context, use store.ToolUse) error {
	if err := s.Store.SaveToolUse(ctx, use); err != nil {
		return err
	}
	if use.Status == s.stage {
		os.Exit(23)
	}
	if s.stage == "reset" && use.Status == store.ToolUseStatusAuthorized {
		os.Exit(23)
	}
	return nil
}

func recoveryTestSession(data store.Store, dir, stage string, safe, recover bool) *Session {
	options := []Option{WithID("ses_recover"), WithRunID("run_recover"), WithStore(data), WithClient(&recoveryClient{dir: dir, stage: stage}), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}), WithTools(tool.NewFuncTool("inspect", "inspect", tool.Definition{Name: "inspect", ReplaySafe: safe, InputSchema: tool.InputSchema{Type: "object"}}, func(ctx context.Context, inv tool.Invocation) (tool.Result, error) {
		if inv.Input["value"] != "kept" {
			return tool.Result{}, errors.New("input changed")
		}
		if err := appendRecoveryMarker(dir, "tools"); err != nil {
			return tool.Result{}, err
		}
		if stage == "effect" || stage == "batch" {
			os.Exit(23)
		}
		return tool.Result{Text: "read result", Structured: map[string]any{"retained": true}, OutputParts: models.Content{models.TextPart{Text: "extra result"}}}, nil
	}))}
	options = append(options, WithPermissions(permission.Ruleset{{Action: "*", Resource: "*", Effect: permission.EffectAsk}}), WithPermissionPrompter(recoveryPrompter{dir: dir}), WithTools(tool.NewFuncTool("write_test", "write", tool.Definition{Name: "write_test", Sequential: true, InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
		return tool.Result{Text: "written"}, appendRecoveryMarker(dir, "writes")
	})))
	if recover {
		options = append(options, WithRunRecovery())
	}
	return New(options...)
}

func appendRecoveryMarker(dir, name string) error {
	file, err := os.OpenFile(filepath.Join(dir, name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString("x")
	return err
}

type recoveryClient struct {
	requestCaptureClient
	dir, stage string
}

type recoveryPrompter struct{ dir string }

func (p recoveryPrompter) Request(context.Context, run.PermissionRequestInfo) (run.PermissionResponse, error) {
	return run.PermissionResponseOnce, appendRecoveryMarker(p.dir, "permissions")
}

func (c *recoveryClient) Stream(_ context.Context, request models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	if err := appendRecoveryMarker(c.dir, "models"); err != nil {
		return nil, err
	}
	if c.stage == "dispatch" {
		os.Exit(23)
	}
	if strings.HasPrefix(c.stage, "retry_") {
		return nil, &models.ProviderError{Category: models.ErrorUnavailable, Provider: "test", Retryable: true, Message: "unavailable"}
	}
	message := models.Message{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "call_inspect", Name: "inspect", Input: map[string]any{"value": "kept"}}}}
	if c.stage == "batch" {
		message.Content = append(models.Content{models.ToolCallPart{CallID: "call_write", Name: "write_test", Input: map[string]any{}}}, message.Content...)
	}
	for _, history := range request.Messages {
		if history.State == models.MessageStateFailed {
			return nil, errors.New("partial answer was sent as completed history")
		}
		for _, part := range history.Content {
			if p, ok := part.(models.ToolPart); ok && p.State == models.ToolStateCompleted && p.Name == "inspect" {
				if p.Output != "read result" || p.Structured == nil || len(p.OutputParts) != 1 {
					return nil, errors.New("recovered result changed")
				}
				message.Content = models.Content{models.TextPart{Text: "done"}}
			}
		}
	}
	stream := models.NewEventStream[models.StreamPart, *models.Message](2)
	if c.stage == "partial" {
		stream.Push(models.TextStartPart{ID: "text"})
		stream.Push(models.TextDeltaPart{ID: "text", Delta: "partial answer"})
	}
	stream.Close(&message, nil)
	return stream, nil
}

func TestRunRecoveryExcludesPartialResponseFromCompaction(t *testing.T) {
	dir := t.TempDir()
	command := exec.Command(os.Args[0], "-test.run=^TestRunRecoveryCrashHelper$")
	command.Env = append(os.Environ(), "WINGMAN_RECOVERY_DIR="+dir, "WINGMAN_RECOVERY_STAGE=partial", "WINGMAN_RECOVERY_SAFE=true")
	output, err := command.CombinedOutput()
	var exited *exec.ExitError
	if !errors.As(err, &exited) || exited.ExitCode() != 23 {
		t.Fatalf("crash helper: %v\n%s", err, output)
	}
	data, err := store.NewSQLiteStore(filepath.Join(dir, "wingman.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	ctx := context.Background()
	if err := data.InterruptActiveModelCalls(ctx, "run_recover", "process_interrupted", "process stopped"); err != nil {
		t.Fatal(err)
	}
	if err := RecoverRunMessages(ctx, data, "ses_recover", "run_recover"); err != nil {
		t.Fatal(err)
	}
	if _, err := data.RequeueSessionRun(ctx, "run_recover"); err != nil {
		t.Fatal(err)
	}
	if _, err := data.ClaimNextSessionRun(ctx, "ses_recover"); err != nil {
		t.Fatal(err)
	}
	client := &recoveryCompactionClient{}
	sess := New(WithID("ses_recover"), WithRunID("run_recover"), WithStore(data), WithRunRecovery(), WithClient(client), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{ContextWindow: 1}), WithPlugin(compaction.New(compaction.WithMinMessages(0))))
	result, err := sess.Run(ctx, "inspect")
	if err != nil || result.Response != "done" || client.summaries != 1 {
		t.Fatalf("compaction recovery = %#v, %v, summaries = %d", result, err, client.summaries)
	}
	request, err := json.Marshal(client.request.Messages)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(request), "partial answer") || !strings.Contains(string(request), "conversation-checkpoint") {
		t.Fatalf("recovered context = %s", request)
	}
	messages, err := data.ListMessages(ctx, "ses_recover")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, message := range messages {
		if message.State == "failed" && len(message.Parts) > 0 && strings.Contains(string(message.Parts[0].PayloadJSON), "partial answer") {
			found = true
		}
	}
	if !found {
		t.Fatal("partial response was removed from durable history")
	}
}

type recoveryCompactionClient struct {
	requestCaptureClient
	summaries int
}

func (c *recoveryCompactionClient) Generate(_ context.Context, request models.Request) (*models.Message, error) {
	c.summaries++
	body, err := json.Marshal(request.Messages)
	if err != nil {
		return nil, err
	}
	if strings.Contains(string(body), "partial answer") {
		return nil, errors.New("partial response reached compaction")
	}
	return &models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "The user requested inspection."}}}, nil
}
