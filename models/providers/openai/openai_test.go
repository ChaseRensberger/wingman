package openai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/models/route"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestCodexRouteOwnsAuthentication(t *testing.T) {
	for _, mode := range []string{"current", "refresh", "expired", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			refreshes, requests := 0, 0
			cfg := provider.RouteConfig{Info: models.ModelInfo{Provider: ID, ID: "test", API: models.APIOpenAIResponses, BaseURL: "https://api.openai.com/v1"}, Credential: provider.Credential{Type: "oauth", Access: "access", AccountID: "account", ExpiresAt: time.Now().Add(time.Hour).Unix()}}
			if mode == "refresh" || mode == "expired" {
				cfg.Credential.ExpiresAt = 1
			}
			if mode == "refresh" {
				cfg.Refresh = func(ctx context.Context, id string, previous provider.Credential) (provider.Credential, error) {
					refreshes++
					if id != ID || previous.Access != "access" {
						t.Errorf("refresh input = %s, %#v", id, previous)
					}
					previous.Access = "refreshed"
					return previous, nil
				}
			}
			if mode == "disabled" {
				enabled := false
				cfg.Options.Auth = &enabled
			}
			deployment, err := newRoute(cfg)
			if err != nil {
				t.Fatal(err)
			}
			deployment.Transport = route.HTTP{Client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				if req.URL.String() != "https://chatgpt.com/backend-api/codex/responses" || req.Header.Get("originator") != "codex_cli_rs" {
					t.Errorf("request = %s, %#v", req.URL, req.Header)
				}
				want := "Bearer access"
				if mode == "refresh" {
					want = "Bearer refreshed"
				}
				if mode == "disabled" {
					want = ""
				}
				if req.Header.Get("Authorization") != want {
					t.Errorf("auth = %q", req.Header.Get("Authorization"))
				}
				if mode != "disabled" && req.Header.Get("ChatGPT-Account-Id") != "account" {
					t.Error("account header missing")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\"}\n\n"))}, nil
			})}}
			model := &route.Model{Info_: cfg.Info, Route: deployment}
			req := models.Request{HTTP: models.HTTPOptions{Body: map[string]any{"store": true}}}
			prepared, err := model.Prepare(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if prepared.Body["store"] != false || refreshes != 0 || requests != 0 || prepared.Headers["authorization"] != "" {
				t.Fatalf("prepare dispatched or exposed auth: %#v", prepared)
			}
			_, err = model.Generate(context.Background(), req)
			if mode == "expired" {
				var failure *models.ProviderError
				if !errors.As(err, &failure) || failure.Category != models.ErrorAuthentication || requests != 0 {
					t.Fatalf("error = %#v, requests = %d", err, requests)
				}
			} else if err != nil || requests != 1 {
				t.Fatalf("error = %v, requests = %d", err, requests)
			}
			if (refreshes == 1) != (mode == "refresh") {
				t.Fatalf("refreshes = %d", refreshes)
			}
		})
	}
}
