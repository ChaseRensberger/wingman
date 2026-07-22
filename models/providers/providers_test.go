package provider_test

import (
	"context"
	"testing"

	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/models/providers/opencodego"
)

func TestOpenCodeGoKimiK3Route(t *testing.T) {
	client := provider.NewClient(map[string]string{opencodego.ID: "test-key"})
	prepared, err := client.Prepare(context.Background(), models.Request{
		Model:    opencodego.Model("kimi-k3"),
		Messages: []models.Message{models.NewUserText("hello")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.URL != "https://opencode.ai/zen/go/v1/chat/completions" {
		t.Errorf("URL = %q", prepared.URL)
	}
	if prepared.API != models.APIOpenAICompatible {
		t.Errorf("API = %q, want %q", prepared.API, models.APIOpenAICompatible)
	}
	if prepared.Body["model"] != "kimi-k3" {
		t.Errorf("model = %v, want kimi-k3", prepared.Body["model"])
	}
}
