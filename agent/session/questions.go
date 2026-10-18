package session

import (
	"context"

	"github.com/chaserensberger/wingman/question"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/tool"
)

// QuestionTool creates a tool backed by the session store's durable questions.
func QuestionTool(data store.Store, publish func(store.SessionEvent)) tool.Tool {
	return tool.NewQuestionTool(func(ctx context.Context, inv tool.Invocation, prompts []question.Prompt) (question.Question, error) {
		transition, err := data.CreateQuestion(ctx, question.Question{SessionID: inv.SessionID, RunID: inv.RunID, ToolUseID: inv.ToolUseID, Questions: prompts})
		if err != nil {
			return question.Question{}, err
		}
		if publish != nil {
			for _, event := range transition.Events {
				publish(event)
			}
		}
		return transition.Question, nil
	})
}

// WithQuestionEventSink observes committed question wait and resume events.
func WithQuestionEventSink(sink func(store.SessionEvent)) Option {
	return func(s *Session) { s.questionEventSink = sink }
}
