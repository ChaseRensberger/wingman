package session

import (
	"context"
	"errors"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/plugins/compaction"
)

type stoppedCompactionClient struct {
	recoveryCompactionClient
	stop func() error
}

func (c *stoppedCompactionClient) Generate(context.Context, models.Request) (*models.Message, error) {
	return nil, c.stop()
}

func TestFailedCompactionPersistsOneTerminalSnapshot(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "provider-failure", true: "cancelled"}[cancelled], func(t *testing.T) {
			data := historyStore(t, "sqlite")
			seedHistory(t, data, 0)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			client := &stoppedCompactionClient{stop: func() error {
				if cancelled {
					cancel()
					return ctx.Err()
				}
				return errors.New("provider unavailable")
			}}
			sess := New(WithID("ses_history"), WithStore(data), WithClient(client), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{ContextWindow: 8192}), WithPlugin(compaction.New()))
			if err := sess.RunAction(ctx, "compaction.compact", nil, nil); err == nil {
				t.Fatal("compaction unexpectedly succeeded")
			}
			messages, err := data.ListMessages(t.Context(), "ses_history")
			if err != nil || len(messages) != 4 {
				t.Fatalf("messages=%#v err=%v", messages, err)
			}
			last := messages[3]
			if last.State != string(models.MessageStateFailed) || last.Revision != 2 || last.Idx != 3 {
				t.Fatalf("terminal checkpoint = %#v", last)
			}
			if len(sess.History()) != 4 {
				t.Fatalf("duplicate lifecycle snapshots in history: %#v", sess.History())
			}
		})
	}
}
