package session_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/agent/loop/looptest"
	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/tool"
)

// TestSessionStart_DirectoryScopedToolsRequireWorkDir asks: does the session
// refuse to start when directory-scoped tools are configured but no workdir is
// set, and does it start fine in all other combinations?
func TestSessionStart_DirectoryScopedToolsRequireWorkDir(t *testing.T) {
	ctx := context.Background()

	t.Run("with workdir and filesystem tools starts fine", func(t *testing.T) {
		model := looptest.NewRecordingModel(looptest.Reply("ok"))
		sess := session.New(
			session.WithModel(model),
			session.WithWorkDir("/tmp"),
			session.WithTools(tool.NewBashTool()),
		)
		_, err := sess.Run(ctx, "hello")
		if err != nil {
			t.Fatalf("expected session to start, got error: %v", err)
		}
	})

	t.Run("without workdir and filesystem tools fails to start", func(t *testing.T) {
		model := looptest.NewRecordingModel(looptest.Reply("ok"))
		sess := session.New(
			session.WithModel(model),
			session.WithTools(tool.NewBashTool()),
		)
		_, err := sess.Run(ctx, "hello")
		if err == nil {
			t.Fatal("expected session to fail starting without workdir")
		}
		if !strings.Contains(err.Error(), "session cannot start") {
			t.Errorf("expected 'session cannot start' error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "bash") {
			t.Errorf("expected error to mention tool name, got: %v", err)
		}
	})

	t.Run("without workdir and only non-filesystem tools starts fine", func(t *testing.T) {
		model := looptest.NewRecordingModel(looptest.Reply("ok"))
		sess := session.New(
			session.WithModel(model),
			session.WithTools(tool.NewWebFetchTool()),
		)
		_, err := sess.Run(ctx, "hello")
		if err != nil {
			t.Fatalf("expected session to start, got error: %v", err)
		}
	})
}

func TestSessionLoggerEmitsLifecycleWithoutContent(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	model := looptest.NewRecordingModel(
		looptest.ReplyWithTool("probe", `{"secret":"do-not-log"}`),
		looptest.Reply("final response should stay out of logs"),
	)
	probe := tool.NewFuncTool("probe", "probe tool", tool.Definition{
		Name:        "probe",
		Description: "probe tool",
		InputSchema: tool.InputSchema{
			Type: "object",
			Properties: map[string]tool.Property{
				"secret": {Type: "string"},
			},
		},
	}, func(ctx context.Context, params map[string]any, workDir string) (string, error) {
		return "tool output should stay out of logs", nil
	})

	sess := session.New(
		session.WithModel(model),
		session.WithTools(probe),
		session.WithLogger(logger),
	)
	result, err := sess.Run(context.Background(), "user prompt should stay out of logs")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if result.StopReason == "" || result.StopReason != "end_turn" {
		t.Fatalf("unexpected stop reason: %q", result.StopReason)
	}

	out := logs.String()
	for _, leaked := range []string{"do-not-log", "tool output should stay out of logs", "user prompt should stay out of logs", "final response should stay out of logs"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("log leaked content %q:\n%s", leaked, out)
		}
	}
	for _, want := range []string{"session run started", "tool execution started", "tool execution completed", "loop turn completed", "session run completed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected log %q in:\n%s", want, out)
		}
	}
	if !strings.Contains(out, `"tool":"probe"`) {
		t.Fatalf("expected tool name in logs:\n%s", out)
	}
	if !strings.Contains(out, `"provider":"looptest"`) || !strings.Contains(out, `"model":"recording-model"`) {
		t.Fatalf("expected provider/model in logs:\n%s", out)
	}
}
