package provider

import (
	"context"
	"testing"

	"github.com/chaserensberger/wingman/models"
)

func TestClientPrepareUsesExplicitRouteForCustomModel(t *testing.T) {
	client := NewClient(nil)
	prepared, err := client.Prepare(context.Background(), models.Request{
		Model: models.ModelRef{
			Provider: "openai",
			ID:       "custom-model",
			API:      models.APIOpenAIResponses,
			BaseURL:  "https://example.test/v1",
		},
		Messages: []models.Message{models.NewUserText("hello")},
	})
	if err != nil {
		t.Fatalf("prepare custom route: %v", err)
	}
	if prepared.URL != "https://example.test/v1/responses" {
		t.Fatalf("url = %q, want custom base URL", prepared.URL)
	}
	if prepared.Body["model"] != "custom-model" {
		t.Fatalf("body model = %v, want custom-model", prepared.Body["model"])
	}
}

func TestClientPrepareRequiresRouteForUnknownModel(t *testing.T) {
	client := NewClient(nil)
	_, err := client.Prepare(context.Background(), models.Request{
		Model:    models.ModelRef{Provider: "openai", ID: "custom-model"},
		Messages: []models.Message{models.NewUserText("hello")},
	})
	if err == nil {
		t.Fatal("prepare unknown model succeeded without api/base_url")
	}
}
