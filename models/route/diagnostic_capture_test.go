package route_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"unicode/utf8"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/protocols"
	"github.com/chaserensberger/wingman/models/route"
)

func TestDiagnosticHTTPContextAndRateLimits(t *testing.T) {
	for _, status := range []int{200, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			const evidence = `{"error":{"type":"insufficient_quota","message":"Add credits","unrecognized":{"detail":"preserved"}}}`
			body := evidence
			if status == 200 {
				body = "event: error\ndata: " + body + "\n\n"
			}
			model := diagnosticModel(protocols.Chat{}, status, nil)
			model.Route.Transport = route.HTTP{Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Header: http.Header{
					"X-Request-Id": {"req_context"}, "Retry-After": {"30"}, "Retry-After-Ms": {"125.5"},
					"X-Ratelimit-Limit-Tokens": {"1000"}, "X-Ratelimit-Remaining-Tokens": {"42"}, "X-Ratelimit-Reset-Tokens": {"1s"},
					"Anthropic-Ratelimit-Requests-Limit": {"50"}, "Anthropic-Ratelimit-Requests-Remaining": {"0"}, "Anthropic-Ratelimit-Requests-Reset": {"2026-09-20T12:00:00Z"},
					"Set-Cookie": {"provider-session=secret-cookie; HttpOnly"}, "X-Api-Key": {"secret-response-key"},
					"Server-Timing": {"model;dur=12", "queue;dur=3"},
				}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}}
			_, err := model.Generate(context.Background(), models.Request{})
			var failure *models.ProviderError
			if !errors.As(err, &failure) || failure.Diagnostic == nil {
				t.Fatalf("failure = %v", err)
			}
			d := failure.Diagnostic
			if failure.Category != models.ErrorQuota || failure.Retryable || failure.RetryAfter == nil || failure.RetryAfter.Microseconds() != 125500 || d.RetryAfterMS != 125 {
				t.Fatalf("failure = %#v", failure)
			}
			if d.Message != "Add credits" || d.Body != evidence || d.RateLimit.Limit["tokens"] != "1000" || d.RateLimit.Remaining["tokens"] != "42" || d.RateLimit.Reset["tokens"] != "1s" || d.RateLimit.Limit["requests"] != "50" || d.RateLimit.Remaining["requests"] != "0" || d.RateLimit.Reset["requests"] != "2026-09-20T12:00:00Z" {
				t.Fatalf("diagnostic = %#v", d)
			}
			if len(d.ResponseHeaders["server-timing"]) != 2 || d.ResponseHeaders["set-cookie"][0] != "[redacted]" || d.ResponseHeaders["x-api-key"][0] != "[redacted]" {
				t.Fatalf("headers = %#v", d.ResponseHeaders)
			}
			if status == 200 && d.Event != "error" {
				t.Fatal("SSE event name lost")
			}
			encoded, _ := json.Marshal(d)
			if strings.Contains(string(encoded), "secret-") {
				t.Fatal("response credential leaked")
			}
		})
	}
}

type diagnosticStackError struct{}

func (diagnosticStackError) Error() string { return "native error with credential-value" }
func (e diagnosticStackError) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, e.Error())
	if s.Flag('+') {
		_, _ = io.WriteString(s, "\nprovider.go:42 credential-value")
	}
}

func TestDiagnosticTransportCauses(t *testing.T) {
	model := diagnosticModel(protocols.Responses{}, 200, nil)
	model.Route.Auth = route.BearerAuth("credential-value")
	native := errors.Join(&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, diagnosticStackError{})
	model.Route.Transport = route.HTTP{Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, native })}}
	_, err := model.Generate(context.Background(), models.Request{})
	var failure *models.ProviderError
	if !errors.As(err, &failure) || failure.Diagnostic == nil || !errors.Is(err, syscall.ECONNREFUSED) {
		t.Fatalf("failure = %v", err)
	}
	d := failure.Diagnostic
	if d.Stage != "request" || d.Transport.Code != "ECONNREFUSED" || d.Transport.Phase != "connect" || d.Transport.Delivery != "not-sent" || d.Transport.Recovery != "retry-full" {
		t.Fatalf("transport = %#v", d.Transport)
	}
	encoded, _ := json.Marshal(d)
	for _, value := range []string{"*url.Error", "*net.OpError", "syscall.Errno", "provider.go:42", "ECONNREFUSED"} {
		if !strings.Contains(string(encoded), value) {
			t.Fatalf("missing %s in %s", value, encoded)
		}
	}
	if strings.Contains(string(encoded), "credential-value") || strings.Contains(failure.Error(), "native error") {
		t.Fatalf("sensitive detail exposed: %s, %s", encoded, failure)
	}
}

func TestDiagnosticDecodingAndReadFailures(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		body                           io.Reader
		category                       models.ErrorCategory
		classification, kind, evidence string
	}{
		{"malformed event", strings.NewReader("event: broken\ndata: {broken}\n\n"), models.ErrorDecoding, "", "event", "{broken}"},
		{"non SSE response", strings.NewReader("<html>proxy returned HTML</html>"), models.ErrorDecoding, "incomplete-stream", "stream", "<html>proxy returned HTML</html>"},
		{"read error", io.MultiReader(strings.NewReader("data: {\"type\":\"response.created\"}\n\n"), brokenDiagnosticStream{}), models.ErrorTransport, "", "stream", "response.created"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := diagnosticModel(protocols.Responses{}, 200, tc.body).Generate(context.Background(), models.Request{})
			var failure *models.ProviderError
			if !errors.As(err, &failure) || failure.Diagnostic == nil {
				t.Fatalf("failure = %v", err)
			}
			d := failure.Diagnostic
			if d.Category != tc.category || d.Classification != tc.classification || d.BodyKind != tc.kind || !strings.Contains(d.Body, tc.evidence) || d.Transport.Recovery != "fail" {
				t.Fatalf("diagnostic = %#v", d)
			}
			if tc.name == "read error" && (d.Transport.Code != "UNEXPECTED_EOF" || len(d.Causes) == 0) {
				t.Fatal("read cause lost")
			}
			if tc.name == "malformed event" && (d.Event != "broken" || len(d.Causes) == 0) {
				t.Fatal("decode cause or event lost")
			}
		})
	}
}

func TestDiagnosticPartialHTTPErrorBody(t *testing.T) {
	body := io.MultiReader(strings.NewReader("upstream proxy failure"), brokenDiagnosticStream{})
	_, err := diagnosticModel(protocols.Responses{}, 502, body).Generate(context.Background(), models.Request{})
	var failure *models.ProviderError
	if !errors.As(err, &failure) || failure.Diagnostic == nil {
		t.Fatalf("failure = %v", err)
	}
	d := failure.Diagnostic
	if d.Body != "upstream proxy failure" || !d.DetailOmitted || len(d.Causes) != 1 || d.Causes[0].Message != "unexpected EOF" || d.Transport.Delivery != "rejected" {
		t.Fatalf("diagnostic = %#v", d)
	}
}

func TestDiagnosticRedactsEscapedAndTruncatedCredentials(t *testing.T) {
	for _, secret := range []string{`credential-"quoted\value`, "credential-with-a-long-tail"} {
		for _, truncated := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", secret, truncated), func(t *testing.T) {
				body, _ := json.Marshal(map[string]any{"error": map[string]any{"message": "Rejected " + secret}, "unknown": secret})
				if truncated {
					body = []byte(strings.Repeat("x", (64<<10)-12) + secret + strings.Repeat("z", 100))
				}
				model := diagnosticModel(protocols.Responses{}, 400, strings.NewReader(string(body)))
				model.Route.Auth = route.HeaderAuth("X-Api-Key", secret)
				_, err := model.Generate(context.Background(), models.Request{})
				var failure *models.ProviderError
				if !errors.As(err, &failure) || failure.Diagnostic == nil {
					t.Fatalf("failure = %v", err)
				}
				d := failure.Diagnostic
				encoded, _ := json.Marshal(d)
				if strings.Contains(string(encoded), "credential-") || !d.Redacted || d.Truncated != truncated || len(d.Body) > 64<<10 || !utf8.ValidString(d.Body) {
					t.Fatalf("redaction/limits failed: bytes=%d redacted=%v truncated=%v", len(d.Body), d.Redacted, d.Truncated)
				}
			})
		}
	}
}

func TestDiagnosticPrepareFailure(t *testing.T) {
	model := diagnosticModel(protocols.Responses{}, 200, nil)
	model.Route.Auth = route.AuthFunc(func(req *http.Request) error {
		req.Header.Set("X-Api-Key", "credential-value")
		return errors.New("credential-value could not authenticate")
	})
	_, err := model.Generate(context.Background(), models.Request{})
	var failure *models.ProviderError
	if !errors.As(err, &failure) || failure.Diagnostic == nil {
		t.Fatalf("failure = %v", err)
	}
	d := failure.Diagnostic
	if d.Stage != "prepare" || d.Transport.Delivery != "not-sent" || d.Causes[0].Message != "[redacted] could not authenticate" {
		t.Fatalf("diagnostic = %#v", d)
	}
}

func TestDiagnosticBearerRedactionPreservesFollowingFields(t *testing.T) {
	body := `{"error":{"message":"Rejected Bearer unrelated-credential"},"extra":{"explanation":"use another endpoint"}}`
	_, err := diagnosticModel(protocols.Responses{}, 400, strings.NewReader(body)).Generate(context.Background(), models.Request{})
	var failure *models.ProviderError
	if !errors.As(err, &failure) || failure.Diagnostic == nil {
		t.Fatalf("failure = %v", err)
	}
	d := failure.Diagnostic
	if !json.Valid([]byte(d.Body)) || strings.Contains(d.Body, "unrelated-credential") || !strings.Contains(d.Body, `"explanation":"use another endpoint"`) || !d.Redacted {
		t.Fatalf("redaction removed evidence: %s", d.Body)
	}
}
