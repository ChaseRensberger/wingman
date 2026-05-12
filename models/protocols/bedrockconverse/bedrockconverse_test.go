package bedrockconverse

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/route"
)

func TestPrepareBuildsBedrockBodyAndEndpoint(t *testing.T) {
	ref := route.ModelRef{Provider: "bedrock", ModelID: "anthropic.claude-3-5-sonnet", BaseURL: "https://bedrock-runtime.us-east-1.amazonaws.com", MaxOutputTokens: 77}
	body, err := (Protocol{}).Prepare(context.Background(), ref, models.Request{
		System:     "sys",
		Messages:   []models.Message{models.NewUserText("hello")},
		Tools:      []models.ToolDef{{Name: "lookup", Description: "Lookup", InputSchema: map[string]any{"type": "object"}}},
		ToolChoice: models.ToolChoice{Mode: models.ToolChoiceRequired},
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body.Body, &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if got["modelId"] != "anthropic.claude-3-5-sonnet" || got["system"] == nil || got["toolConfig"] == nil {
		t.Fatalf("unexpected body: %#v", got)
	}
	url, err := Endpoint().URL(ref)
	if err != nil {
		t.Fatalf("Endpoint.URL() error = %v", err)
	}
	if url != "https://bedrock-runtime.us-east-1.amazonaws.com/model/anthropic.claude-3-5-sonnet/converse-stream" {
		t.Fatalf("url = %q", url)
	}
}
