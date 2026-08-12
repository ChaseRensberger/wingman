package compaction

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
)

func TestTransformHistoryUsesPreparedRequestBudget(t *testing.T) {
	client := &testClient{summary: "summary", body: map[string]any{"system": strings.Repeat("x", 400)}}
	p := New(WithMinMessages(0), WithKeepTail(1), WithReserveTokens(20))
	msgs := []models.Message{
		{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "old"}}},
		{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "recent"}}},
	}
	out, err := p.transformHistory(context.Background(), run.TransformHistoryInfo{
		Messages: msgs, Client: client, Model: models.ModelRef{Provider: "test", ID: "model"},
		ModelInfo: models.ModelInfo{ContextWindow: 100, MaxOutput: 10},
		Request:   models.Request{Model: models.ModelRef{Provider: "test", ID: "model"}, Messages: msgs},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 || findLatestMarker(out) != 1 {
		t.Fatalf("history = %#v, want marker before recent message", out)
	}
	if client.maxOutput != 10 {
		t.Fatalf("summary max output = %d, want 10", client.maxOutput)
	}
}

func TestTransformHistoryRejectsEmptySummary(t *testing.T) {
	client := &testClient{body: map[string]any{"messages": strings.Repeat("x", 400)}}
	p := New(WithMinMessages(0), WithKeepTail(1), WithReserveTokens(20))
	msgs := []models.Message{
		{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "old"}}},
		{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "recent"}}},
	}
	_, err := p.transformHistory(context.Background(), run.TransformHistoryInfo{
		Messages: msgs, Client: client, Model: models.ModelRef{Provider: "test", ID: "model"},
		ModelInfo: models.ModelInfo{ContextWindow: 100},
		Request:   models.Request{Model: models.ModelRef{Provider: "test", ID: "model"}, Messages: msgs},
	})
	if err == nil || !strings.Contains(err.Error(), "empty summary") {
		t.Fatalf("error = %v, want empty summary error", err)
	}
}

func TestTransformHistoryIgnoresOversizedReserve(t *testing.T) {
	client := &testClient{summary: "summary", body: map[string]any{"messages": "small"}}
	p := New(WithMinMessages(0), WithKeepTail(1))
	msgs := []models.Message{
		{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "old"}}},
		{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "recent"}}},
	}
	out, err := p.transformHistory(context.Background(), run.TransformHistoryInfo{
		Messages: msgs, Client: client, Model: models.ModelRef{Provider: "test", ID: "model"},
		ModelInfo: models.ModelInfo{ContextWindow: 100},
		Request:   models.Request{Model: models.ModelRef{Provider: "test", ID: "model"}, Messages: msgs},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(msgs) {
		t.Fatalf("history count = %d, want %d", len(out), len(msgs))
	}
}

func TestSummaryOutputTokensFitsSmallContext(t *testing.T) {
	if got := summaryOutputTokens(models.ModelInfo{ContextWindow: 8}); got != 2 {
		t.Fatalf("summary output tokens = %d, want 2", got)
	}
}

type testClient struct {
	summary   string
	body      map[string]any
	maxOutput int
}

func (c *testClient) Prepare(context.Context, models.Request) (*models.PreparedRequest, error) {
	return &models.PreparedRequest{Body: c.body}, nil
}

func (c *testClient) Generate(_ context.Context, req models.Request) (*models.Message, error) {
	c.maxOutput = req.MaxOutputTokens
	return &models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: c.summary}}}, nil
}

func (*testClient) Stream(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	return nil, errors.New("unexpected Stream")
}
