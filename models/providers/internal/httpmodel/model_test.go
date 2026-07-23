package httpmodel

import (
	"testing"

	"github.com/chaserensberger/wingman/models"
)

func TestParseOpenAIChatUsesChoiceUsage(t *testing.T) {
	state := parseState{}
	stream := models.NewEventStream[models.StreamPart, *models.Message](1)

	parseOpenAIChat(map[string]any{
		"choices": []any{
			map[string]any{
				"usage": map[string]any{
					"prompt_tokens":     float64(120),
					"completion_tokens": float64(30),
					"total_tokens":      float64(150),
				},
			},
		},
	}, &state, stream)

	if state.usage.InputTokens != 120 || state.usage.OutputTokens != 30 || state.usage.TotalTokens != 150 {
		t.Fatalf("usage = %#v, want usage from choices[0]", state.usage)
	}
}

func TestParseOpenAIChatKeepsPriorUsage(t *testing.T) {
	state := parseState{}
	stream := models.NewEventStream[models.StreamPart, *models.Message](1)

	parseOpenAIChat(map[string]any{
		"usage": map[string]any{
			"prompt_tokens":     float64(120),
			"completion_tokens": float64(30),
			"total_tokens":      float64(150),
		},
	}, &state, stream)
	parseOpenAIChat(map[string]any{}, &state, stream)

	if state.usage.TotalTokens != 150 {
		t.Fatalf("total tokens = %d, want prior usage retained", state.usage.TotalTokens)
	}
}

func TestOpenAIChatBodyExpandsCanonicalToolPart(t *testing.T) {
	model := &Model{Protocol: OpenAIChat, Info_: models.ModelInfo{ID: "test"}}
	body, err := model.body(models.Request{Messages: []models.Message{{
		Role: models.RoleAssistant,
		Content: models.Content{models.ToolPart{
			CallID: "call_1",
			Name:   "bash",
			State:  models.ToolStateCompleted,
			Input:  map[string]any{"command": "pwd"},
			Output: "/tmp",
		}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	messages := body["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages = %#v, want assistant tool call and tool result", messages)
	}
	if messages[0].(map[string]any)["role"] != "assistant" || messages[1].(map[string]any)["role"] != "tool" {
		t.Fatalf("messages = %#v", messages)
	}
}
