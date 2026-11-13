package provider

import (
	"context"
	"testing"

	"github.com/chaserensberger/wingman/models"
)

type registeredClient struct{ credential Credential }

func (c *registeredClient) Prepare(_ context.Context, request models.Request) (*models.PreparedRequest, error) {
	return &models.PreparedRequest{Model: request.Model, Metadata: map[string]any{"credential": c.credential}}, nil
}

func (c *registeredClient) Stream(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	stream := models.NewEventStream[models.StreamPart, *models.Message](0)
	stream.Close(&models.Message{Role: models.RoleAssistant}, nil)
	return stream, nil
}

func (c *registeredClient) Generate(ctx context.Context, request models.Request) (*models.Message, error) {
	return models.Generate(ctx, c, request)
}

func TestProviderRegistrationSnapshotAndCredentialIsolation(t *testing.T) {
	base, err := NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	registration := Registration{Meta: ProviderMeta{ID: "native", Name: "Native"}, Models: map[string]models.ModelInfo{"echo": {ID: "echo", Env: []string{"WINGMAN_NATIVE_TEST_KEY"}, Variants: []models.ModelVariant{{ID: "fast"}}}}, NewClient: func(credential Credential, _ ProviderOptions) models.Client {
		return &registeredClient{credential: credential}
	}}
	registry, err := base.WithRegistrations([]Registration{registration})
	if err != nil {
		t.Fatal(err)
	}
	if base.IsValid("native") {
		t.Fatal("registration changed existing registry")
	}
	info := registration.Models["echo"]
	info.Variants[0].ID = "changed"
	info.Env[0] = "CHANGED"
	if info, _ := registry.Catalog().Get("native", "echo"); info.Variants[0].ID != "fast" || info.Env[0] != "WINGMAN_NATIVE_TEST_KEY" {
		t.Fatal("catalog retained mutable registration data")
	}
	if cfg, _ := registry.Config("native"); cfg.Models["echo"].Variants[0].ID != "fast" {
		t.Fatal("config retained mutable registration data")
	}
	t.Setenv("WINGMAN_NATIVE_TEST_KEY", "environment-key")
	client := registry.NewClientWithCredentials(map[string]Credential{"other": {Type: "api_key", Key: "other-secret"}}, nil)
	request := models.Request{Model: models.ModelRef{Provider: "native", ID: "echo", Variant: "fast"}}
	prepared, err := client.Prepare(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if credential := prepared.Metadata["credential"].(Credential); credential.Key != "environment-key" {
		t.Fatalf("wrong provider credential: %#v", credential)
	}
	for _, ref := range []models.ModelRef{{Provider: "native", ID: "missing"}, {Provider: "native", ID: "echo", Variant: "missing"}} {
		if _, err := client.Prepare(t.Context(), models.Request{Model: ref}); err == nil {
			t.Fatalf("Prepare accepted %#v", ref)
		}
		if _, err := client.Stream(t.Context(), models.Request{Model: ref}); err == nil {
			t.Fatalf("Stream accepted %#v", ref)
		}
	}
	if _, err := base.WithRegistrations([]Registration{registration, registration}); err == nil {
		t.Fatal("duplicate provider registrations succeeded")
	}
}

func TestNativeProviderDisabledAuthDoesNotResolveCredentials(t *testing.T) {
	t.Setenv("WINGMAN_NATIVE_DISABLED_KEY", "environment-secret")
	for _, source := range []string{"stored", "auth", "environment"} {
		t.Run(source, func(t *testing.T) {
			auth := false
			base, err := NewRegistry(map[string]ProviderConfig{"native": {Options: ProviderOptions{Auth: &auth}}})
			if err != nil {
				t.Fatal(err)
			}
			registration := Registration{Meta: ProviderMeta{ID: "native"}, Models: map[string]models.ModelInfo{"echo": {ID: "echo", Env: []string{"WINGMAN_NATIVE_DISABLED_KEY"}}}, NewClient: func(credential Credential, options ProviderOptions) models.Client {
				if credential != (Credential{}) {
					t.Errorf("disabled auth received %#v", credential)
				}
				if options.Auth == nil || *options.Auth {
					t.Error("disabled authentication setting was not forwarded")
				}
				return &registeredClient{}
			}}
			registry, err := base.WithRegistrations([]Registration{registration})
			if err != nil {
				t.Fatal(err)
			}
			client := registry.NewClient(nil)
			if source == "stored" {
				client.Credentials = map[string]Credential{"native": {Type: "api_key", Key: "stored-secret"}}
			}
			if source == "auth" {
				client.Auth = map[string]string{"native": "auth-secret"}
			}
			request := models.Request{Model: models.ModelRef{Provider: "native", ID: "echo"}}
			if _, err := client.Prepare(t.Context(), request); err != nil {
				t.Fatal(err)
			}
			if _, err := client.Generate(t.Context(), request); err != nil {
				t.Fatal(err)
			}
		})
	}
}
