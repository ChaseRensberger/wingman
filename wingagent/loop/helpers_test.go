package loop_test

import (
	"testing"

	"github.com/chaserensberger/wingman/wingagent/loop"
	"github.com/chaserensberger/wingman/wingagent/loop/looptest"
	"github.com/chaserensberger/wingman/wingmodels"
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

func hasText(msg wingmodels.Message, text string) bool {
	for _, p := range msg.Content {
		if tp, ok := p.(wingmodels.TextPart); ok && tp.Text == text {
			return true
		}
	}
	return false
}

func containsMessageWithText(msgs []wingmodels.Message, text string) bool {
	for _, m := range msgs {
		if hasText(m, text) {
			return true
		}
	}
	return false
}
