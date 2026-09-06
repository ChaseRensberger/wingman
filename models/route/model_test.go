package route_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/protocols"
	"github.com/chaserensberger/wingman/models/route"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func testModel(status int, headers http.Header) *route.Model {
	return &route.Model{Info_: models.ModelInfo{Provider: "test", ID: "test"}, Route: route.Route{
		Protocol: protocols.Chat{}, Endpoint: route.Endpoint{BaseURL: "https://example.com"}, Framing: route.SSE,
		Transport: route.HTTP{Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader("data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))}, nil
		})}},
	}}
}

func TestStreamEmitsProviderRequestID(t *testing.T) {
	for _, tt := range []struct {
		name    string
		headers http.Header
		id      string
	}{
		{"priority", http.Header{"X-Request-Id": {"preferred"}, "Request-Id": {"fallback"}, "Openai-Request-Id": {"openai"}, "X-Goog-Request-Id": {"google"}}, "preferred"},
		{"request", http.Header{"Request-Id": {"anthropic"}}, "anthropic"},
		{"openai", http.Header{"Openai-Request-Id": {"openai"}}, "openai"},
		{"google", http.Header{"X-Goog-Request-Id": {"google"}}, "google"},
		{"absent", http.Header{"Cf-Ray": {"not-request-id"}}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stream, err := testModel(200, tt.headers).Stream(context.Background(), models.Request{})
			if err != nil {
				t.Fatal(err)
			}
			var parts []models.StreamPart
			for part := range stream.Iter() {
				parts = append(parts, part)
			}
			if _, err := stream.Final(); err != nil {
				t.Fatal(err)
			}
			if _, ok := parts[0].(models.StreamStartPart); !ok {
				t.Fatalf("first part = %T", parts[0])
			}
			metadata := 0
			for i, part := range parts {
				if p, ok := part.(models.ResponseMetadataPart); ok {
					metadata++
					if i != 1 || len(p.Meta) != 1 || p.Meta["request_id"] != tt.id {
						t.Fatalf("metadata = %#v", p)
					}
				}
			}
			if (metadata == 1) != (tt.id != "") {
				t.Fatalf("metadata count = %d", metadata)
			}
		})
	}
}

func TestResponseErrorClassification(t *testing.T) {
	for _, tt := range []struct {
		status   int
		category models.ErrorCategory
		retry    bool
	}{
		{401, models.ErrorAuthentication, false}, {403, models.ErrorAuthorization, false}, {408, models.ErrorTimeout, true},
		{429, models.ErrorRateLimit, true}, {400, models.ErrorInvalidRequest, false}, {503, models.ErrorUnavailable, true},
	} {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			_, err := testModel(tt.status, http.Header{"X-Request-Id": {"request-failed"}, "Retry-After": {"2"}}).Stream(context.Background(), models.Request{})
			var failure *models.ProviderError
			if !errors.As(err, &failure) || failure.Category != tt.category || failure.Retryable != tt.retry || failure.RequestID != "request-failed" || failure.Status != tt.status {
				t.Fatalf("failure = %#v", err)
			}
			if failure.RetryAfter == nil || *failure.RetryAfter != 2*time.Second {
				t.Fatalf("retry after = %v", failure.RetryAfter)
			}
		})
	}
	_, err := testModel(429, http.Header{"Retry-After": {time.Now().Add(time.Second).Format(http.TimeFormat)}}).Stream(context.Background(), models.Request{})
	var failure *models.ProviderError
	if !errors.As(err, &failure) || failure.RetryAfter == nil || *failure.RetryAfter < 0 {
		t.Fatalf("date retry after = %#v", failure)
	}
}

func TestRequestOptionsAndQueryMerge(t *testing.T) {
	configured := map[string]string{"configured": "yes", "same": "old"}
	variant := map[string]any{"nested": map[string]any{"variant": true, "same": "variant"}}
	model := testModel(200, nil)
	model.Info_.Provider = "openai"
	model.Route.Endpoint.Query = configured
	model.Variant = models.ModelVariant{ProviderOptions: variant, HTTP: models.HTTPOptions{Headers: map[string]string{"x-choice": "variant"}, Body: map[string]any{"model": "variant-http"}}}
	req := models.Request{ProviderOptions: models.ProviderBag{"openai": {"temperature": 0.3, "model": "provider", "nested": map[string]any{"same": "request"}}}, HTTP: models.HTTPOptions{Headers: map[string]string{"x-choice": "request"}, Body: map[string]any{"model": "caller"}, Query: map[string]string{"same": "new", "request": "yes"}}}
	prepared, err := model.Prepare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Body["temperature"] != 0.3 || prepared.Body["model"] != "caller" || prepared.Headers["x-choice"] != "request" {
		t.Fatalf("prepared = %#v", prepared)
	}
	if prepared.URL != "https://example.com?configured=yes&request=yes&same=new" || configured["same"] != "old" {
		t.Fatalf("URL = %s, configured = %#v", prepared.URL, configured)
	}
	if variant["nested"].(map[string]any)["same"] != "variant" {
		t.Fatal("variant was mutated")
	}
	if nested := prepared.Body["nested"].(map[string]any); nested["same"] != "request" || nested["variant"] != true {
		t.Fatalf("nested = %#v", nested)
	}
}

func TestSSEFraming(t *testing.T) {
	var frames []route.Frame
	for frame, err := range route.SSE(context.Background(), strings.NewReader(": comment\r\nevent: native\r\ndata: first\r\ndata: second\r\n\r\ndata: [DONE]")) {
		if err != nil {
			t.Fatal(err)
		}
		frames = append(frames, frame)
	}
	if len(frames) != 2 || frames[0].Event != "native" || frames[0].Data != "first\nsecond" || frames[1].Data != "[DONE]" {
		t.Fatalf("frames = %#v", frames)
	}
}

func TestHeaderOverridesAreCaseInsensitive(t *testing.T) {
	model := testModel(200, nil)
	model.Variant.HTTP.Headers = map[string]string{"CONTENT-TYPE": "variant", "X-Choice": "variant", "X-Route": "variant"}
	model.Route.Headers = map[string]string{"X-ROUTE": "route"}
	req := models.Request{HTTP: models.HTTPOptions{Headers: map[string]string{"Content-Type": "application/vnd.test+json", "x-choice": "request", "x-route": "request"}}}
	for range 100 {
		prepared, err := model.Prepare(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if len(prepared.Headers) != 3 || prepared.Headers["content-type"] != "application/vnd.test+json" || prepared.Headers["x-choice"] != "request" || prepared.Headers["x-route"] != "route" {
			t.Fatalf("headers = %#v", prepared.Headers)
		}
		model.Route.Transport = route.HTTP{Client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Header.Get("Content-Type") != "application/vnd.test+json" || r.Header.Get("X-Choice") != "request" || r.Header.Get("X-Route") != "route" {
				t.Errorf("wire headers = %#v", r.Header)
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n"))}, nil
		})}}
		if _, err := model.Generate(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	if model.Variant.HTTP.Headers["CONTENT-TYPE"] != "variant" || req.HTTP.Headers["Content-Type"] != "application/vnd.test+json" {
		t.Fatal("input headers were mutated")
	}
}

func TestSSESkipsEmptyPayloadsAtEOF(t *testing.T) {
	for _, body := range []string{"data:", "data: ", "data:\n\n"} {
		for frame, err := range route.SSE(context.Background(), strings.NewReader(body)) {
			t.Fatalf("unexpected frame = %#v, error = %v", frame, err)
		}
	}
}
