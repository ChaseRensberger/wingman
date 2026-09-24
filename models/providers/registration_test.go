package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/chaserensberger/wingman/models"
)

func TestMissingOAuthRouteRegistration(t *testing.T) {
	registry, err := NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	// Other tests import OpenAI; this isolated generation models a caller that does not.
	delete(registry.routes, "openai")
	client := registry.NewClientWithCredentials(map[string]Credential{"openai": {Type: "oauth", Access: "private-token"}}, nil)
	req := models.Request{Model: models.ModelRef{Provider: "openai", ID: "gpt-6-luna"}}
	prepared, err := client.Prepare(context.Background(), req)
	var failure *models.ProviderError
	if prepared != nil || !errors.As(err, &failure) || failure.Category != models.ErrorAuthentication || failure.Message != "provider OAuth route is not registered" {
		t.Fatalf("prepared = %#v, error = %v", prepared, err)
	}
	if stream, err := client.Stream(context.Background(), req); stream != nil || !errors.As(err, &failure) {
		t.Fatalf("stream = %v, error = %v", stream, err)
	}

	prepared, err = registry.NewClient(map[string]string{"openai": "test-key"}).Prepare(context.Background(), req)
	if err != nil || prepared.URL != "https://api.openai.com/v1/responses" {
		t.Fatalf("API key default route = %#v, error = %v", prepared, err)
	}

	auth := false
	registry.configs["openai"] = ProviderConfig{Options: ProviderOptions{Auth: &auth}}
	if _, err := client.Prepare(context.Background(), req); err != nil {
		t.Fatalf("disabled authentication failed: %v", err)
	}
}
