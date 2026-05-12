package openairesponses

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/route"
)

func TestPrepareBuildsResponsesBody(t *testing.T) {
	body, err := (Protocol{}).Prepare(context.Background(), route.ModelRef{Provider: "openai", ModelID: "gpt-4o", MaxOutputTokens: 123, Info: models.ModelInfo{Capabilities: models.ModelCapabilities{Reasoning: true}}}, models.Request{
		System:       "sys",
		Messages:     []models.Message{models.NewUserText("hello")},
		Tools:        []models.ToolDef{{Name: "lookup", Description: "Lookup", InputSchema: map[string]any{"type": "object"}}},
		ToolChoice:   models.ToolChoice{Mode: models.ToolChoiceRequired},
		Capabilities: models.Capabilities{Thinking: &models.ThinkingConfig{Effort: "high"}},
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body.Body, &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if got["model"] != "gpt-4o" || got["stream"] != true || got["tool_choice"] != "required" {
		t.Fatalf("unexpected body: %#v", got)
	}
	if got["max_output_tokens"] != float64(123) {
		t.Fatalf("max_output_tokens = %#v", got["max_output_tokens"])
	}
	if got["reasoning"] == nil {
		t.Fatalf("missing reasoning config: %#v", got)
	}
}
