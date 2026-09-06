// Package openai composes OpenAI API and Codex deployments.
package openai

import (
	"fmt"
	"net/http"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/models/route"
)

// ID is the provider ID for OpenAI model refs.
const ID = "openai"

// Model returns an OpenAI model ref.
func Model(id string) models.ModelRef {
	return models.ModelRef{Provider: ID, ID: id}
}

func init() {
	provider.Register(provider.ProviderMeta{
		ID:        ID,
		Name:      "OpenAI",
		AuthTypes: []provider.AuthType{{Type: "oauth", Name: "ChatGPT Pro/Plus"}, {Type: "api_key"}},
	}, newRoute)
}

func newRoute(cfg provider.RouteConfig) (route.Route, error) {
	deployment, err := provider.DefaultRoute(cfg)
	if err != nil {
		return route.Route{}, err
	}
	if cfg.Credential.Type != "oauth" {
		return deployment, nil
	}
	deployment.Endpoint.BaseURL = "https://chatgpt.com/backend-api/codex"
	deployment.Headers = route.MergeStrings(deployment.Headers, map[string]string{"originator": "codex_cli_rs"})
	deployment.Body = func(body map[string]any) map[string]any { body["store"] = false; return body }
	if cfg.Options.Auth != nil && !*cfg.Options.Auth {
		return deployment, nil
	}
	deployment.Auth = route.AuthFunc(func(req *http.Request) error {
		current := cfg.Credential
		if current.Access == "" || current.ExpiresAt <= time.Now().Unix() {
			if cfg.Refresh == nil {
				return fmt.Errorf("openai OAuth token is expired; reconnect the provider")
			}
			var err error
			current, err = cfg.Refresh(req.Context(), cfg.Info.Provider, current)
			if err != nil {
				return err
			}
		}
		if current.Access == "" {
			return fmt.Errorf("openai OAuth access token is missing")
		}
		req.Header.Set("authorization", "Bearer "+current.Access)
		if current.AccountID != "" {
			req.Header.Set("ChatGPT-Account-Id", current.AccountID)
		}
		return nil
	})
	return deployment, nil
}
