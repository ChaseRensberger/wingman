package session

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestPersistModelCallStoresUsageUnavailableAndEstimatedCost(t *testing.T) {
	data := memory.NewStore()
	stored := &store.Session{ID: "ses_test"}
	if err := data.CreateSession(stored); err != nil {
		t.Fatal(err)
	}
	sess := New(WithID(stored.ID), WithStore(data))
	model := models.ModelRef{Provider: "test", ID: "model"}
	info := models.ModelInfo{InputCostPerMTok: 3, OutputCostPerMTok: 15}

	if err := sess.persistModelCall(context.Background(), "msg_zero", 1, models.Message{}, model, info, ""); err != nil {
		t.Fatal(err)
	}
	if err := sess.persistModelCall(context.Background(), "msg_used", 2, models.Message{
		Usage: &models.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
	}, model, info, ""); err != nil {
		t.Fatal(err)
	}

	calls, err := data.ListModelCalls(context.Background(), stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("call count = %d, want 2", len(calls))
	}
	if calls[0].TotalTokens != 0 || calls[0].Cost != nil {
		t.Fatalf("usage-unavailable call = %#v, want zero usage and no cost", calls[0])
	}
	if calls[1].Cost == nil || math.Abs(*calls[1].Cost-18) > 0.000001 {
		t.Fatalf("cost = %v, want 18", calls[1].Cost)
	}
}

func TestFinalizeUnsettledToolsMarksCallsAsInterrupted(t *testing.T) {
	messages := []models.Message{{Role: models.RoleAssistant, Content: models.Content{models.ToolPart{
		CallID: "call_1",
		Name:   "bash",
		State:  models.ToolStateRunning,
		Input:  map[string]any{"command": "pwd"},
	}}}}

	finalizeUnsettledTools(messages)
	tool := messages[0].Content[0].(models.ToolPart)
	if tool.State != models.ToolStateError || tool.Error != "Tool execution interrupted" || tool.Output != "" || tool.CompletedAt == 0 {
		t.Fatalf("tool = %#v, want interrupted error", tool)
	}
}

func TestRunFinalizesInterruptedToolsBeforeRequest(t *testing.T) {
	client := &requestCaptureClient{}
	sess := New(
		WithClient(client),
		WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}),
	)
	sess.SetHistory([]models.Message{{Role: models.RoleAssistant, Content: models.Content{models.ToolPart{
		CallID: "call_1",
		Name:   "bash",
		State:  models.ToolStatePending,
		Input:  map[string]any{"command": "pwd"},
	}}}})

	if _, err := sess.Run(context.Background(), "continue"); err != nil {
		t.Fatal(err)
	}
	if len(client.request.Messages) != 2 {
		t.Fatalf("request messages = %#v", client.request.Messages)
	}
	tool := client.request.Messages[0].Content[0].(models.ToolPart)
	if tool.State != models.ToolStateError || tool.Error != "Tool execution interrupted" {
		t.Fatalf("tool = %#v, want interrupted error", tool)
	}
	if expanded := models.ExpandToolMessages(client.request.Messages); len(expanded) != 3 || expanded[1].Role != models.RoleTool {
		t.Fatalf("expanded request = %#v, want assistant call followed by tool result", expanded)
	}
}

type requestCaptureClient struct {
	request models.Request
}

func (c *requestCaptureClient) Prepare(context.Context, models.Request) (*models.PreparedRequest, error) {
	return nil, errors.New("unexpected Prepare")
}

func (c *requestCaptureClient) Generate(context.Context, models.Request) (*models.Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (c *requestCaptureClient) Stream(_ context.Context, request models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	c.request = request
	stream := models.NewEventStream[models.StreamPart, *models.Message](0)
	stream.Close(&models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "done"}}}, nil)
	return stream, nil
}
