package loop_test

import (
	"testing"

	"github.com/chaserensberger/wingman/agent/loop"
	"github.com/chaserensberger/wingman/agent/loop/looptest"
	"github.com/chaserensberger/wingman/models"
)

func newConfig(t *testing.T, model *looptest.RecordingModel, sink *looptest.RecordingSink, hooks loop.Hooks) loop.Config {
	t.Helper()
	return loop.Config{
		Model:    model,
		Sink:     sink,
		Hooks:    hooks,
		MaxSteps: 5,
	}
}

func hasText(msg models.Message, text string) bool {
	for _, p := range msg.Content {
		if tp, ok := p.(models.TextPart); ok && tp.Text == text {
			return true
		}
	}
	return false
}

func containsMessageWithText(msgs []models.Message, text string) bool {
	for _, m := range msgs {
		if hasText(m, text) {
			return true
		}
	}
	return false
}
