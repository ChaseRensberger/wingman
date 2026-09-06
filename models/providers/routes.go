package provider

import (
	"context"
	"fmt"
	"net/url"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/protocols"
	"github.com/chaserensberger/wingman/models/route"
)

// RouteConfig supplies resolved model and authentication inputs to a provider facade.
type RouteConfig struct {
	Info       models.ModelInfo
	APIKey     string
	Credential Credential
	Options    ProviderOptions
	Refresh    func(context.Context, string, Credential) (Credential, error)
}

type routeDefaults struct {
	protocol   route.Protocol
	path       func(string) string
	authHeader string
	headers    map[string]string
	query      map[string]string
}

var supportedRoutes = map[models.API]routeDefaults{
	models.APIOpenAIResponses:   {protocol: protocols.Responses{}, path: func(string) string { return "/responses" }},
	models.APIOpenAICompletions: {protocol: protocols.Chat{}, path: func(string) string { return "/chat/completions" }},
	models.APIOpenAICompatible:  {protocol: protocols.Chat{}, path: func(string) string { return "/chat/completions" }},
	models.APIAnthropicMessages: {
		protocol: protocols.Anthropic{}, path: func(string) string { return "/messages" }, authHeader: "x-api-key",
		headers: map[string]string{"anthropic-version": "2023-06-01", "anthropic-beta": "interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14"},
	},
	models.APIGeminiGenerate: {
		protocol: protocols.Gemini{}, path: func(model string) string { return "/models/" + url.PathEscape(model) + ":streamGenerateContent" },
		authHeader: "x-goog-api-key", query: map[string]string{"alt": "sse"},
	},
}

func protocolFor(api models.API) (route.Protocol, error) {
	defaults, ok := supportedRoutes[api]
	if !ok {
		return nil, fmt.Errorf("unsupported model API: %s", api)
	}
	return defaults.protocol, nil
}

// DefaultRoute composes supported API defaults for direct and compatible deployments.
func DefaultRoute(cfg RouteConfig) (route.Route, error) {
	protocol, err := protocolFor(cfg.Info.API)
	if err != nil {
		return route.Route{}, err
	}
	defaults := supportedRoutes[cfg.Info.API]
	auth := route.NoAuth
	if cfg.APIKey != "" && (cfg.Options.Auth == nil || *cfg.Options.Auth) {
		header := cfg.Options.AuthHeader
		if header == "" {
			header = defaults.authHeader
		}
		if header == "" {
			auth = route.BearerAuth(cfg.APIKey)
		} else {
			value := cfg.APIKey
			if cfg.Options.AuthScheme != "" {
				value = cfg.Options.AuthScheme + " " + value
			}
			auth = route.HeaderAuth(header, value)
		}
	}
	return route.Route{
		ID: protocol.ID(), Protocol: protocol,
		Endpoint: route.Endpoint{BaseURL: cfg.Info.BaseURL, Path: defaults.path(cfg.Info.ID), Query: route.MergeStrings(defaults.query, cfg.Options.Query)},
		Auth:     auth, Headers: route.MergeStrings(nil, defaults.headers), Transport: route.HTTP{}, Framing: route.SSE,
	}, nil
}
