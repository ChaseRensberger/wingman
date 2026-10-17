package session

import (
	"context"
	"errors"
	"testing"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/tool"
)

type admissionClient struct{ requestCaptureClient }

func (*admissionClient) Stream(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	stream := models.NewEventStream[models.StreamPart, *models.Message](0)
	stream.Close(&models.Message{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "call", Name: "test"}}}, nil)
	return stream, nil
}

type admissionStore struct {
	store.Store
	authorized chan struct{}
}

func (s *admissionStore) SaveToolUse(ctx context.Context, use store.ToolUse) error {
	if err := s.Store.SaveToolUse(ctx, use); err != nil {
		return err
	}
	if use.Status == store.ToolUseStatusAuthorized {
		s.authorized <- struct{}{}
	}
	return nil
}

func TestCanceledToolAdmissionSettlesDurableUse(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			data := &admissionStore{Store: historyStore(t, backend), authorized: make(chan struct{}, 1)}
			limiter := run.NewToolLimiter(1)
			holdCtx, stopHolder := context.WithCancel(t.Context())
			defer stopHolder()
			holding, holderDone := make(chan struct{}), make(chan error, 1)
			definition := tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object"}}
			model := models.ModelRef{Provider: "test", ID: "model"}
			go func() {
				_, err := run.Run(holdCtx, run.Config{Client: &admissionClient{}, Model: model, MaxSteps: 1, ToolLimiter: limiter, Tools: []tool.Tool{tool.NewFuncTool("test", "test", definition, func(ctx context.Context, _ tool.Invocation) (tool.Result, error) {
					close(holding)
					<-ctx.Done()
					return tool.Result{}, ctx.Err()
				})}})
				holderDone <- err
			}()
			<-holding
			admitted, err := data.AdmitSessionRun(t.Context(), store.SessionRun{SessionID: "ses_history", Agent: store.Agent{ID: "agt_test"}, Message: "go"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := data.ClaimNextSessionRun(t.Context(), "ses_history"); err != nil {
				t.Fatal(err)
			}
			sess := New(WithID("ses_history"), WithRunID(admitted.Run.ID), WithStore(data), WithClient(&admissionClient{}), WithModelRef(model, models.ModelInfo{}), WithToolLimits(4, limiter), WithTools(tool.NewFuncTool("test", "test", definition, func(context.Context, tool.Invocation) (tool.Result, error) {
				t.Error("canceled tool executed")
				return tool.Result{}, nil
			})))
			defer sess.Close(context.Background())
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := sess.Run(ctx, "go"); done <- err }()
			<-data.authorized
			cancel()
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			uses, err := data.ListRunToolUses(t.Context(), "ses_history", admitted.Run.ID)
			if err != nil || len(uses) != 1 {
				t.Fatalf("uses=%#v err=%v", uses, err)
			}
			if uses[0].Status != store.ToolUseStatusInterrupted || !uses[0].StartedAt.IsZero() || uses[0].CompletedAt.IsZero() {
				t.Fatalf("use=%#v", uses[0])
			}
			messages, err := data.QueryMessages(t.Context(), "ses_history", store.MessageQuery{RunID: admitted.Run.ID, Role: "assistant"})
			if err != nil || len(messages) != 1 {
				t.Fatalf("messages=%#v err=%v", messages, err)
			}
			message, err := StoredMessageToModel(messages[0])
			if err != nil {
				t.Fatal(err)
			}
			if message.Content[0].(models.ToolPart).State != models.ToolStateError {
				t.Fatalf("message=%#v", message)
			}
			stopHolder()
			<-holderDone
		})
	}
}
