package session_test

import (
	"context"
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
