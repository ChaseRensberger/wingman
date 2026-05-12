package anthropicmessages

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/route"
)

func TestPrepareBuildsAnthropicBodyAndHeaders(t *testing.T) {
	body, err := (Protocol{}).Prepare(context.Background(), route.ModelRef{Provider: "anthropic", ModelID: "claude-3-7-sonnet", MaxOutputTokens: 100}, models.Request{
		System:       "sys",
		Messages:     []models.Message{models.NewUserText("hello")},
		Tools:        []models.ToolDef{{Name: "lookup", Description: "Lookup", InputSchema: map[string]any{"type": "object"}}},
		ToolChoice:   models.ToolChoice{Mode: models.ToolChoiceTool, Tool: "lookup"},
		Capabilities: models.Capabilities{Thinking: &models.ThinkingConfig{BudgetTokens: 64}},
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body.Body, &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if got["model"] != "claude-3-7-sonnet" || got["stream"] != true || got["max_tokens"] != float64(100) {
		t.Fatalf("unexpected body: %#v", got)
	}
	if body.Headers.Get("anthropic-version") == "" || body.Headers.Get("anthropic-beta") == "" {
		t.Fatalf("missing anthropic headers: %#v", body.Headers)
	}
}
