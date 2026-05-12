package gemini

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/route"
)

func TestPrepareBuildsGeminiBodyAndEndpoint(t *testing.T) {
	ref := route.ModelRef{Provider: "google", ModelID: "gemini-2.5-pro", BaseURL: "https://generativelanguage.googleapis.com/v1beta", MaxOutputTokens: 99}
	body, err := (Protocol{}).Prepare(context.Background(), ref, models.Request{
		System:     "sys",
		Messages:   []models.Message{models.NewUserText("hello")},
		Tools:      []models.ToolDef{{Name: "lookup", Description: "Lookup", InputSchema: map[string]any{"type": "object"}}},
		ToolChoice: models.ToolChoice{Mode: models.ToolChoiceTool, Tool: "lookup"},
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body.Body, &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if got["systemInstruction"] == nil || got["toolConfig"] == nil || got["generationConfig"] == nil {
		t.Fatalf("missing gemini fields: %#v", got)
	}
	url, err := Endpoint().URL(ref)
	if err != nil {
		t.Fatalf("Endpoint.URL() error = %v", err)
	}
	if url != "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse" {
		t.Fatalf("url = %q", url)
	}
}
