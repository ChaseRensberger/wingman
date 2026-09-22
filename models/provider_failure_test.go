package models

import (
	"strings"
	"testing"
)

func TestClassifyProviderFailure(t *testing.T) {
	for _, tc := range []struct {
		name           string
		status         int
		body           string
		category       ErrorCategory
		classification string
		retry          bool
	}{
		{"OpenAI credits", 429, `{"error":{"code":"credit_balance_exhausted","type":"insufficient_quota","message":"Add credits"}}`, ErrorQuota, "", false},
		{"quota precedence", 429, `{"code":"rate_limit_exceeded","error":{"type":"insufficient_quota"}}`, ErrorQuota, "", false},
		{"Zen quota", 429, `{"error":{"code":"GoUsageLimitError"}}`, ErrorQuota, "", false},
		{"gateway label", 429, `{"error":{"message":"Provider: [CreditLimitExceeded] Account cap"}}`, ErrorQuota, "", false},
		{"quota text", 429, `{"error":{"message":"Your usage limit was reached"}}`, ErrorQuota, "", false},
		{"billing HTTP", 402, "Payment required", ErrorQuota, "", false},
		{"Anthropic throttle", 429, `{"error":{"type":"rate_limit_error"}}`, ErrorRateLimit, "", true},
		{"Gemini throttle", 200, `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED"}}`, ErrorRateLimit, "", true},
		{"Bedrock throttle", 0, `{"exception":{"type":"ThrottlingException"}}`, ErrorRateLimit, "", true},
		{"throttle text", 0, `{"message":"Rate increased too quickly"}`, ErrorRateLimit, "", true},
		{"context code", 400, `{"error":{"code":"context_length_exceeded"}}`, ErrorInvalidRequest, "context-overflow", false},
		{"context native text", 400, `{"error":{"type":"invalid_request_error","message":"prompt is too long: 210000 tokens"}}`, ErrorInvalidRequest, "context-overflow", false},
		{"context nested evidence", 400, `{"error":{"message":"Rejected","metadata":{"detail":"input length exceeds context length"}}}`, ErrorInvalidRequest, "context-overflow", false},
		{"empty body status explanation", 400, "400 (no body)", ErrorInvalidRequest, "context-overflow", false},
		{"no false context on 500", 500, `{"message":"maximum context length is 100 tokens"}`, ErrorUnavailable, "", true},
		{"token rate not context", 429, `{"message":"rate limit: too many tokens"}`, ErrorRateLimit, "", true},
		{"oversized payload", 413, "Request Entity Too Large", ErrorInvalidRequest, "payload-too-large", false},
		{"provider request too large", 413, `{"error":{"type":"request_too_large","message":"request_too_large"}}`, ErrorInvalidRequest, "payload-too-large", false},
		{"Azure policy", 400, `{"error":{"innererror":{"code":"ResponsibleAIPolicyViolation"}}}`, ErrorContentPolicy, "", false},
		{"OpenRouter policy", 400, `{"error":{"metadata":{"error_type":"content_policy_violation"}}}`, ErrorContentPolicy, "", false},
		{"Anthropic policy", 400, `{"error":{"type":"invalid_request_error","message":"blocked by content filtering policy"}}`, ErrorContentPolicy, "", false},
		{"invalid prompt policy", 400, `{"error":{"code":"invalid_prompt","message":"violating our usage policy"}}`, ErrorContentPolicy, "", false},
		{"invalid prompt schema", 400, `{"error":{"code":"invalid_prompt","message":"bad schema"}}`, ErrorInvalidRequest, "", false},
		{"policy response type", 0, `{"response":{"error_type":"image_content_policy_violation"}}`, ErrorContentPolicy, "", false},
		{"auth", 401, "no credentials", ErrorAuthentication, "", false},
		{"authorization", 403, "access denied", ErrorAuthorization, "", false},
		{"Gemini auth", 0, `{"error":{"status":"UNAUTHENTICATED"}}`, ErrorAuthentication, "", false},
		{"nested known type", 0, `{"code":"new_code","error":{"code":"newer_code","type":"authentication_error"}}`, ErrorAuthentication, "", false},
		{"overload", 0, `{"error":{"type":"overloaded_error"}}`, ErrorUnavailable, "", true},
		{"server text", 0, `{"message":"Provider is temporarily at capacity"}`, ErrorUnavailable, "", true},
		{"gateway server code on 400", 400, `{"error":{"code":"server_error","message":"bad input"}}`, ErrorInvalidRequest, "", false},
		{"Bedrock validation with retry text", 0, `{"exception":{"type":"ValidationException","message":"Correct input and try again"}}`, ErrorInvalidRequest, "", false},
		{"conflict", 409, "conflict", ErrorUnavailable, "", true},
		{"timeout", 408, "timeout", ErrorTimeout, "", true},
		{"proxy HTML", 502, "<html>upstream failed</html>", ErrorUnavailable, "", true},
		{"unrecognized 4xx", 422, `{"novel":{"explanation":"bad input"}}`, ErrorInvalidRequest, "", false},
		{"unknown stream", 0, `{"type":"error","error":{"code":"future_failure","message":"new error"}}`, ErrorProvider, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ClassifyProviderFailure("test", tc.status, tc.body)
			if err.Category != tc.category || err.Classification != tc.classification || err.Retryable != tc.retry {
				t.Fatalf("got %s/%s retry=%v, want %s/%s retry=%v", err.Category, err.Classification, err.Retryable, tc.category, tc.classification, tc.retry)
			}
			if err.Diagnostic.Body != tc.body || err.Status != tc.status {
				t.Fatal("original evidence changed")
			}
		})
	}
}

func TestProviderFailureRetainsMixedEnvelope(t *testing.T) {
	body := `{"type":"error","code":"top_code","message":"native message","error":{"type":"insufficient_quota","param":"text.format","extra":{"unknown":"preserved"}}}`
	failure := ClassifyProviderFailure("test", 200, body)
	d := failure.Diagnostic
	if d.Code != "top_code" || d.Message != "native message" || d.Type != "insufficient_quota" || d.Param != "text.format" || d.Body != body || failure.Category != ErrorQuota {
		t.Fatalf("failure = %#v, diagnostic = %#v", failure, d)
	}
	if strings.Contains(failure.Error(), "native message") {
		t.Fatal("native prose leaked into public summary")
	}
}
