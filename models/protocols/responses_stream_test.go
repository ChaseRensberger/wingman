package protocols

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/route"
)

func responsesTestModel(body io.Reader) *route.Model {
	return &route.Model{
		Info_: models.ModelInfo{Provider: "openai", ID: "test", API: models.APIOpenAIResponses},
		Route: route.Route{Protocol: Responses{}, Endpoint: route.Endpoint{BaseURL: "https://provider.test", Path: "/responses"}, Framing: route.SSE,
			Transport: route.HTTP{Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"X-Request-Id": {"req_stream"}},
					Body:       io.NopCloser(body),
				}, nil
			})}}},
	}
}

func TestResponsesStreamFailures(t *testing.T) {
	tests := []struct {
		name     string
		event    string
		category models.ErrorCategory
		retry    bool
	}{
		{"nested response error", `{"type":"response.failed","response":{"error":{"code":"server_error","message":"private upstream detail"}}}`, models.ErrorUnavailable, true},
		{"nested error", `{"type":"error","error":{"code":"rate_limit_exceeded","message":"private upstream detail"}}`, models.ErrorRateLimit, true},
		{"top level error", `{"type":"error","code":"invalid_api_key","message":"private upstream detail"}`, models.ErrorAuthentication, false},
		{"error under response", `{"type":"error","response":{"error":{"code":"context_length_exceeded","message":"private upstream detail"}}}`, models.ErrorInvalidRequest, false},
		{"error type", `{"type":"error","error":{"type":"authorization_error","message":"private upstream detail"}}`, models.ErrorAuthorization, false},
		{"quota", `{"type":"response.failed","response":{"error":{"code":"insufficient_quota"}}}`, models.ErrorRateLimit, false},
		{"unknown code", `{"type":"response.failed","response":{"error":{"code":"private upstream detail"}}}`, models.ErrorProvider, false},
		{"missing details", `{"type":"response.failed","response":{"id":"resp_failed"}}`, models.ErrorProvider, false},
		{"null details", `{"type":"error","code":null,"message":null,"error":null}`, models.ErrorProvider, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := responsesTestModel(strings.NewReader("data: " + tt.event + "\n\n"))
			stream, err := model.Stream(context.Background(), models.Request{})
			if err != nil {
				t.Fatal(err)
			}
			var errorParts int
			for part := range stream.Iter() {
				switch part := part.(type) {
				case models.FinishPart:
					t.Fatal("provider failure emitted a successful finish")
				case models.ErrorPart:
					errorParts++
					if strings.Contains(part.Error, "private upstream detail") {
						t.Fatalf("error part exposed provider data: %s", part.Error)
					}
				}
			}
			msg, err := stream.Final()
			var failure *models.ProviderError
			if !errors.As(err, &failure) || failure.Category != tt.category || failure.Retryable != tt.retry {
				t.Fatalf("error = %#v, want %s (retry %v)", err, tt.category, tt.retry)
			}
			if errorParts != 1 || msg.FinishReason != "" {
				t.Fatalf("error parts = %d, message = %#v", errorParts, msg)
			}
			if failure.Status != http.StatusOK || failure.RequestID != "req_stream" {
				t.Fatalf("missing HTTP context: %#v", failure)
			}
			if failure.Cause == nil || failure.Cause.Error() != tt.event {
				t.Fatalf("original provider frame not retained: %#v", failure.Cause)
			}
			if strings.Contains(err.Error(), "private upstream detail") {
				t.Fatalf("public error exposed provider data: %v", err)
			}
		})
	}
}

func TestResponsesStreamRequiresCompletion(t *testing.T) {
	const delta = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"
	tests := []struct {
		name string
		body string
		text string
	}{
		{"empty body", "", ""},
		{"done sentinel", "data: [DONE]\n\n", ""},
		{"created only", "data: {\"type\":\"response.created\"}\n\n", ""},
		{"partial text", delta, "partial"},
		{"partial text and done", delta + "data: [DONE]\n\n", "partial"},
		{"unrecognized event", "data: {\"type\":\"provider.notification\"}\n\n", ""},
		{"non SSE body", "{\"error\":\"not a stream\"}", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := responsesTestModel(strings.NewReader(tt.body))
			stream, err := model.Stream(context.Background(), models.Request{})
			if err != nil {
				t.Fatal(err)
			}
			for part := range stream.Iter() {
				if _, ok := part.(models.FinishPart); ok {
					t.Fatal("unterminated stream emitted a successful finish")
				}
			}
			msg, err := stream.Final()
			var failure *models.ProviderError
			if !errors.As(err, &failure) || failure.Category != models.ErrorDecoding || failure.RequestID != "req_stream" {
				t.Fatalf("error = %#v, want decoding error with request ID", err)
			}
			if msg.FinishReason != "" || joinText(msg.Content) != tt.text {
				t.Fatalf("message = %#v, want unfinished text %q", msg, tt.text)
			}
		})
	}
}

func TestResponsesStreamCompletion(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		finish models.FinishReason
	}{
		{"empty completion", "data: {\"type\":\"response.completed\"}\n\n", models.FinishReasonStop},
		{"completion at EOF", "data: {\"type\":\"response.completed\"}", models.FinishReasonStop},
		{"incomplete output", "data: {\"type\":\"response.incomplete\",\"response\":{\"incomplete_details\":{\"reason\":\"max_output_tokens\"}}}\n\n", models.FinishReasonMaxTokens},
		{"ignore trailing data", "data: {\"type\":\"response.completed\"}\n\ndata: invalid\n\n", models.FinishReasonStop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := responsesTestModel(strings.NewReader(tt.body))
			msg, err := model.Generate(context.Background(), models.Request{})
			if err != nil || msg.FinishReason != tt.finish {
				t.Fatalf("message = %#v, error = %v", msg, err)
			}
		})
	}
}

func TestResponsesStreamFailurePreservesPartialText(t *testing.T) {
	model := responsesTestModel(strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\ndata: {\"type\":\"response.failed\"}\n\n"))
	stream, err := model.Stream(context.Background(), models.Request{})
	if err != nil {
		t.Fatal(err)
	}
	for range stream.Iter() {
	}
	msg, err := stream.Final()
	if err == nil || joinText(msg.Content) != "partial" || msg.FinishReason != "" {
		t.Fatalf("message = %#v, error = %v", msg, err)
	}
}

type afterCompletionReader struct{}

func (afterCompletionReader) Read([]byte) (int, error) {
	return 0, errors.New("read past completion event")
}

func TestResponsesStreamStopsReadingAfterCompletion(t *testing.T) {
	body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":2,\"output_tokens\":1,\"total_tokens\":3}}}\n\n"
	reader := io.MultiReader(strings.NewReader(body), afterCompletionReader{})
	msg, err := responsesTestModel(reader).Generate(context.Background(), models.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if joinText(msg.Content) != "hello" || msg.FinishReason != models.FinishReasonStop || msg.Usage == nil || msg.Usage.TotalTokens != 3 {
		t.Fatalf("message = %#v, want completed text and usage", msg)
	}
}
