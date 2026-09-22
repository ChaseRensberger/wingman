package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/protocols"
	"github.com/chaserensberger/wingman/models/route"
	"github.com/chaserensberger/wingman/store"
)

func TestModelCallDiagnosticsSurviveRestart(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-Id", "req_failure")
		w.Header().Set("X-Ratelimit-Remaining-Tokens", "42")
		w.Header().Set("Set-Cookie", "session=credential-value")
		_, _ = w.Write([]byte("data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"unsupported_model\",\"message\":\"luna needs a different API. Input: private prompt; credential-value\"},\"output\":\"private raw output\"}}\n\n"))
	}))
	defer upstream.Close()
	path := filepath.Join(t.TempDir(), "diagnostics.db")
	data, err := store.NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	if err := data.CreateSession(&store.Session{ID: "ses_diagnostic"}); err != nil {
		t.Fatal(err)
	}
	model := &route.Model{
		Info_: models.ModelInfo{Provider: "openai", ID: "luna", API: models.APIOpenAIResponses},
		Route: route.Route{ID: "openai", Protocol: protocols.Responses{}, Framing: route.SSE, Auth: route.BearerAuth("credential-value"),
			Endpoint: route.Endpoint{BaseURL: upstream.URL, Path: "/responses"}, Transport: route.HTTP{}},
	}
	sess := New(WithID("ses_diagnostic"), WithStore(data), WithClient(model), WithModelRef(models.ModelRef{Provider: "openai", ID: "luna", API: models.APIOpenAIResponses}, model.Info()))
	if _, err := sess.Run(context.Background(), "Please inspect private prompt and summarize the result."); err == nil {
		t.Fatal("expected provider failure")
	}
	if err := data.Close(); err != nil {
		t.Fatal(err)
	}
	data, err = store.NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	// Rebuilding also proves that aggregate history retains the diagnostic.
	if err := data.RebuildSessionProjections(context.Background(), "ses_diagnostic"); err != nil {
		t.Fatal(err)
	}
	calls, err := data.ListModelCalls(context.Background(), "ses_diagnostic")
	if err != nil || len(calls) != 1 {
		t.Fatalf("calls = %#v, error = %v", calls, err)
	}
	call := calls[0]
	var trace models.CallTrace
	if err := json.Unmarshal(call.Trace, &trace); err != nil {
		t.Fatal(err)
	}
	if call.Status != store.ModelCallStatusFailed || call.ProviderRequestID != "req_failure" || trace.Failure == nil || trace.Failure.Code != "unsupported_model" || trace.Failure.HTTPStatus != 200 {
		t.Fatalf("call = %#v, trace = %#v", call, trace)
	}
	if trace.Failure.Message != "luna needs a different API. Input: private prompt; [redacted]" || !trace.Failure.Redacted || trace.Build == nil || trace.Failure.RateLimit.Remaining["tokens"] != "42" || trace.Failure.Transport.Delivery != "accepted" {
		t.Fatalf("diagnostic = %#v, build = %#v", trace.Failure, trace.Build)
	}
	if trace.Timing == nil || trace.Timing.FirstResponseMS == nil || trace.Retry == nil || trace.Retry.Reason != "established_stream" {
		t.Fatalf("attempt timing or retry decision lost after restart: timing = %#v, retry = %#v", trace.Timing, trace.Retry)
	}
	encoded, _ := json.Marshal(call)
	if strings.Contains(string(encoded), "credential-value") || !strings.Contains(string(encoded), "private raw output") || strings.Contains(call.ErrorMessage, "private prompt") {
		t.Fatalf("lost evidence or exposed credential/public detail: %s", encoded)
	}
}

func TestRetriedModelCallDiagnosticsSurviveRestart(t *testing.T) {
	var attempts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Content-Length", "10000")
			w.Header().Set("X-Request-Id", "req_throttle")
			w.Header().Set("Retry-After-Ms", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"type":"rate_limit_error","message":"Too many requests"}}`))
			return
		}
		w.Header().Set("X-Request-Id", "req_quota")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"insufficient_quota","message":"Add credits"}}`))
	}))
	defer upstream.Close()
	path := filepath.Join(t.TempDir(), "retry.db")
	data, err := store.NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	if err := data.CreateSession(&store.Session{ID: "ses_retry_diagnostic"}); err != nil {
		t.Fatal(err)
	}
	model := &route.Model{Info_: models.ModelInfo{Provider: "openai", ID: "luna", API: models.APIOpenAIResponses},
		Route: route.Route{ID: "openai", Protocol: protocols.Responses{}, Framing: route.SSE,
			Endpoint: route.Endpoint{BaseURL: upstream.URL, Path: "/responses"}, Transport: route.HTTP{}}}
	sess := New(WithID("ses_retry_diagnostic"), WithStore(data), WithClient(model),
		WithModelRef(models.ModelRef{Provider: "openai", ID: "luna", API: models.APIOpenAIResponses}, model.Info()),
		WithRetryPolicy(run.RetryPolicy{MaxAttempts: 3, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond}))
	if _, err := sess.Run(context.Background(), "hello"); err == nil {
		t.Fatal("expected quota error")
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d, quota must stop retries", attempts.Load())
	}
	if err := data.Close(); err != nil {
		t.Fatal(err)
	}
	data, err = store.NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.RebuildSessionProjections(context.Background(), "ses_retry_diagnostic"); err != nil {
		t.Fatal(err)
	}
	calls, err := data.ListModelCalls(context.Background(), "ses_retry_diagnostic")
	if err != nil || len(calls) != 2 {
		t.Fatalf("calls = %#v, error = %v", calls, err)
	}
	for _, call := range calls {
		var trace models.CallTrace
		if err := json.Unmarshal(call.Trace, &trace); err != nil {
			t.Fatal(err)
		}
		d := trace.Failure
		if d == nil || d.Body == "" || d.HTTPStatus != 429 || d.Stage != "response" {
			t.Fatalf("missing evidence: %#v", d)
		}
		switch call.Attempt {
		case 1:
			if trace.Retry == nil || trace.Retry.Decision != "scheduled" || trace.Retry.DelayMS == nil || *trace.Retry.DelayMS != 1 {
				t.Fatalf("retry decision lost after restart: %#v", trace.Retry)
			}
			if !d.DetailOmitted || len(d.Causes) != 1 || d.Causes[0].Message != "unexpected EOF" {
				t.Fatalf("native cause did not survive restart: %#v", d)
			}
			if d.Category != models.ErrorRateLimit || !d.Retryable || d.RequestID != "req_throttle" || d.Message != "Too many requests" || d.RetryAfterMS != 1 {
				t.Fatalf("throttle = %#v", d)
			}
		case 2:
			if trace.Retry == nil || trace.Retry.Decision != "not_retried" || trace.Retry.Reason != "ineligible" {
				t.Fatalf("quota decision lost after restart: %#v", trace.Retry)
			}
			if d.Category != models.ErrorQuota || d.Retryable || d.RequestID != "req_quota" || d.Message != "Add credits" {
				t.Fatalf("quota = %#v", d)
			}
		default:
			t.Fatalf("unexpected attempt %d", call.Attempt)
		}
	}
}
