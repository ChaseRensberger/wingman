package provider_test

import (
	"context"
	"testing"

	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/models/providers/openai"
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

func TestOpenAIOAuthUsesCodexRoute(t *testing.T) {
	client := provider.NewClientWithCredentials(map[string]provider.Credential{
		openai.ID: {Type: "oauth", Access: "access", Refresh: "refresh", ExpiresAt: 1},
	}, nil, nil)
	prepared, err := client.Prepare(context.Background(), models.Request{
		Model:    openai.Model("gpt-5.6-terra"),
		Messages: []models.Message{models.NewUserText("hello")},
		HTTP:     models.HTTPOptions{Body: map[string]any{"store": true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.URL != "https://chatgpt.com/backend-api/codex/responses" {
		t.Errorf("URL = %q", prepared.URL)
	}
	if prepared.Headers["originator"] != "codex_cli_rs" {
		t.Errorf("originator = %q", prepared.Headers["originator"])
	}
	if prepared.Body["store"] != false {
		t.Errorf("store = %#v, want false", prepared.Body["store"])
	}
}

func TestClientReportsOpenAIReasoningSummaryLowering(t *testing.T) {
	client := provider.NewClient(map[string]string{openai.ID: "test-key"})
	opts := client.LoweredOptions(context.Background(), models.Request{
		Model:        openai.Model("gpt-5.6-terra"),
		Capabilities: models.Capabilities{Thinking: true},
	})
	if !opts.ReasoningSummaryAuto {
		t.Fatal("ReasoningSummaryAuto = false, want true")
	}
}
