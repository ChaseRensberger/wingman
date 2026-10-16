package session

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/tool"
)

func TestTruncatedToolCallsStayDeclinedAfterProcessCrash(t *testing.T) {
	for _, stage := range []string{"truncated_model", "truncated_declined"} {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			command := exec.Command(os.Args[0], "-test.run=^TestRunRecoveryCrashHelper$")
			command.Env = append(os.Environ(), "WINGMAN_RECOVERY_DIR="+dir, "WINGMAN_RECOVERY_STAGE="+stage, "WINGMAN_RECOVERY_SAFE=true")
			output, err := command.CombinedOutput()
			var exited *exec.ExitError
			if !errors.As(err, &exited) || exited.ExitCode() != 23 {
				t.Fatalf("crash helper: %v\n%s", err, output)
			}
			data, err := store.NewSQLiteStore(filepath.Join(dir, "wingman.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = data.Close() })
			ctx := t.Context()
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
			if _, err := data.ClaimNextSessionRun(ctx, "ses_recover"); err != nil {
				t.Fatal(err)
			}
			sess := recoveryTestSession(data, dir, "", true, true)
			t.Cleanup(func() { _ = sess.Close(ctx) })
			result, err := sess.Run(ctx, "inspect")
			if err != nil || result.Response != "done" {
				t.Fatalf("recovery = %#v, %v", result, err)
			}
			for _, name := range []string{"tools", "permissions", "writes"} {
				if data, err := os.ReadFile(filepath.Join(dir, name)); !os.IsNotExist(err) {
					t.Fatalf("unexpected %s side effect: %q, %v", name, data, err)
				}
			}
			if err := data.RebuildSessionProjections(ctx, "ses_recover"); err != nil {
				t.Fatal(err)
			}
			uses, err := data.ListRunToolUses(ctx, "ses_recover", "run_recover")
			if err != nil || len(uses) != 1 || uses[0].Status != store.ToolUseStatusDeclined || uses[0].ErrorType != "output_token_limit" || !uses[0].AuthorizedAt.IsZero() || !uses[0].StartedAt.IsZero() {
				t.Fatalf("tool records = %#v, %v", uses, err)
			}
			reopened := New(WithID("ses_recover"), WithStore(data))
			t.Cleanup(func() { _ = reopened.Close(ctx) })
			if err := reopened.hydrate(ctx); err != nil {
				t.Fatal(err)
			}
			history := reopened.History()
			if len(history) != 3 || history[1].FinishReason != models.FinishReasonMaxTokens {
				t.Fatalf("history = %#v", history)
			}
			part := history[1].Content[0].(models.ToolPart)
			if part.State != models.ToolStateError || !strings.Contains(part.Error, "output token limit") || part.Input["value"] != "kept" {
				t.Fatalf("tool part = %#v", part)
			}
		})
	}
}

func TestTruncatedAuthorizedToolRecoveryPreservesSavedArguments(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			data := historyStore(t, backend)
			ctx := t.Context()
			if _, err := data.AdmitSessionRun(ctx, store.SessionRun{ID: "run_truncated", SessionID: "ses_history", Message: "write"}); err != nil {
				t.Fatal(err)
			}
			if _, err := data.ClaimNextSessionRun(ctx, "ses_history"); err != nil {
				t.Fatal(err)
			}
			seed := New(WithID("ses_history"), WithRunID("run_truncated"), WithStore(data))
			t.Cleanup(func() { _ = seed.Close(context.Background()) })
			if _, err := seed.persistMessage(ctx, models.Message{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "write"}}}, 0); err != nil {
				t.Fatal(err)
			}
			message, err := seed.persistMessage(ctx, models.Message{Role: models.RoleAssistant, State: models.MessageStateInProgress, FinishReason: models.FinishReasonMaxTokens, Content: models.Content{models.ToolPart{CallID: "call_write", Name: "write", State: models.ToolStatePending, Input: map[string]any{"text": "original"}}}}, 1)
			if err != nil {
				t.Fatal(err)
			}
			if err := data.UpsertModelCall(ctx, store.ModelCall{ID: "mcl_truncated", SessionID: "ses_history", RunID: "run_truncated", AssistantMessageID: message.ID, Step: 1, Status: store.ModelCallStatusCompleted, FinishReason: string(models.FinishReasonMaxTokens)}); err != nil {
				t.Fatal(err)
			}
			use := store.ToolUse{ID: "tlu_truncated", SessionID: "ses_history", RunID: "run_truncated", ModelCallID: "mcl_truncated", AssistantMessageID: message.ID, PartID: models.PartID(message.Content[0]), Step: 1, Ordinal: 1, CallID: "call_write", Name: "write", Status: store.ToolUseStatusProposed, InputJSON: []byte(`{"text":"original"}`), ProposedAt: time.Now()}
			if err := data.SaveToolUse(ctx, use); err != nil {
				t.Fatal(err)
			}
			use.Status, use.AuthorizedAt, use.InputJSON = store.ToolUseStatusAuthorized, time.Now(), []byte(`{"text":"rewritten"}`)
			if err := data.SaveToolUse(ctx, use); err != nil {
				t.Fatal(err)
			}
			if err := data.InterruptActiveToolUses(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := data.RequeueSessionRun(ctx, "run_truncated"); err != nil {
				t.Fatal(err)
			}
			if _, err := data.ClaimNextSessionRun(ctx, "ses_history"); err != nil {
				t.Fatal(err)
			}
			client := &requestCaptureClient{}
			executions := 0
			sess := New(WithID("ses_history"), WithRunID("run_truncated"), WithStore(data), WithRunRecovery(), WithClient(client), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}), WithTools(tool.NewFuncTool("write", "write", tool.Definition{Name: "write", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
				executions++
				return tool.Result{}, nil
			})))
			t.Cleanup(func() { _ = sess.Close(context.Background()) })
			result, err := sess.Run(ctx, "write")
			if err != nil || result.StopReason != run.StopReasonEndTurn || executions != 0 {
				t.Fatalf("result=%#v err=%v executions=%d", result, err, executions)
			}
			uses, err := data.ListRunToolUses(ctx, "ses_history", "run_truncated")
			if err != nil || len(uses) != 1 || uses[0].Status != store.ToolUseStatusFailed || uses[0].ErrorType != "output_token_limit" || string(uses[0].InputJSON) != string(use.InputJSON) || !uses[0].StartedAt.IsZero() || !uses[0].AuthorizedAt.Equal(use.AuthorizedAt) {
				t.Fatalf("uses=%#v err=%v", uses, err)
			}
			part := client.request.Messages[1].Content[0].(models.ToolPart)
			if part.State != models.ToolStateError || part.Input["text"] != "rewritten" || !strings.Contains(part.Error, "output token limit") {
				t.Fatalf("replayed tool = %#v", part)
			}
		})
	}
}
