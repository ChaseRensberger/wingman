package models

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

var contextOverflowPattern = regexp.MustCompile(`(?i)prompt is too long|input is too long for requested model|exceeds the context window|exceeds (the )?(model'?s )?maximum context length|input token count.*exceeds the maximum|tokens in request more than max tokens allowed|maximum prompt length is [0-9]+|reduce the length of the messages|maximum context length is [0-9]+ tokens|exceeds the maximum allowed input length|input \([0-9]+ tokens\) is longer than the model|exceeds the limit of [0-9]+|exceeds the available context size|greater than the context length|context window exceeds limit|exceeded model token limit|context[_ ]length[_ ]exceeded|context length is only [0-9]+ tokens|input length.*exceeds.*context length|prompt too long|too large for model with [0-9]+ maximum context length|prompt has [0-9,]+ tokens?, but the configured context size is|model_context_window_exceeded|range of input length should be|too many tokens|token limit exceeded|request_too_large|^4(00|13)\s*(status code)?\s*\(no body\)`)
var rateLimitPattern = regexp.MustCompile(`(?i)rate increased too quickly|rate[-_\s]?limit|too[_\s]?many[_\s]?requests`)
var quotaPattern = regexp.MustCompile(`(?i)insufficient[-_\s]?quota|quota[-_\s]?exceeded|budget exceeded|usage limit`)
var policyPattern = regexp.MustCompile(`(?i)violating our usage policy|blocked by content filtering policy|content[-_\s]?policy|rejected as a result of our safety system`)
var serverPattern = regexp.MustCompile(`(?i)\b(try again|(please |you can )?retry (the |this |your )?request|try (the |this |your )?request again|(currently |temporarily )?at capacity|overloaded|temporarily unavailable|service[-_\s]?unavailable|(server|internal)[-_\s]?error|server (is )?busy|provider returned (an )?error|resource[-_\s]?exhausted|upstream (connect|connection|request)|request buffer limit while retrying upstream)\b`)
var gatewayCodePattern = regexp.MustCompile(`^[^:\n]+: \[([A-Za-z0-9_.-]+)\]`)

// ClassifyProviderFailure retains native evidence and returns a concise public error.
// Body and diagnostic fields require credential redaction before persistence or export.
func ClassifyProviderFailure(provider string, status int, body string) *ProviderError {
	d, codes := providerFailureDetails(body)
	failure := &ProviderError{Provider: provider, Status: status, Category: ErrorProvider, Message: "provider response failed", Retryable: true, Diagnostic: d}
	if body != "" {
		failure.Cause = errors.New(body)
	}
	// Numeric stream codes describe the rejection even when HTTP established a stream.
	if status < 400 {
		for _, code := range codes {
			if n, err := strconv.Atoi(code); err == nil && n >= 400 && n <= 599 {
				status = n
				break
			}
		}
	}
	has := func(values ...string) bool {
		for _, code := range codes {
			for _, value := range values {
				if code == value {
					return true
				}
			}
		}
		return false
	}
	text := body
	if d.Message != "" {
		text = d.Message + "\n" + body
	}
	lower := strings.ToLower(text)
	clientScoped := status < 500
	set := func(category ErrorCategory, message string, retry bool) {
		failure.Category, failure.Message, failure.Retryable = category, message, retry
	}
	switch {
	case status == 413 || has("request_too_large") || strings.Contains(lower, "request entity too large") || strings.Contains(lower, "payload too large") || strings.Contains(lower, "request too large"):
		set(ErrorInvalidRequest, "provider request payload too large", false)
		failure.Classification = "payload-too-large"
	case clientScoped && (has("context_length_exceeded", "model_context_window_exceeded") ||
		(!strings.Contains(lower, "rate limit") && !strings.Contains(lower, "too many requests") && !strings.HasPrefix(strings.ToLower(d.Message), "throttling error:") && !strings.HasPrefix(strings.ToLower(d.Message), "service unavailable:") && contextOverflowPattern.MatchString(text))):
		set(ErrorInvalidRequest, "provider context window exceeded", false)
		failure.Classification = "context-overflow"
	case has("content_filter", "responsibleaipolicyviolation", "content_policy_violation", "image_content_policy_violation", "refusal") || clientScoped && policyPattern.MatchString(d.Message):
		set(ErrorContentPolicy, "provider content policy rejected the request", false)
	case status == 402 || has("insufficient_quota", "credit_balance_exhausted", "usage_not_included", "billing_error", "gousagelimiterror", "freeusagelimiterror", "creditlimitexceeded") || status == 429 && quotaPattern.MatchString(text):
		set(ErrorQuota, "provider quota exceeded", false)
	case status == 401 || has("invalid_api_key", "authentication_error", "unauthenticated"):
		set(ErrorAuthentication, "provider authentication failed", false)
	case status == 403 || has("permission_error", "permission_denied", "authorization_error"):
		set(ErrorAuthorization, "provider access denied", false)
	case status == 429 || has("resource_exhausted", "too_many_requests", "throttlingexception") || strings.Contains(strings.Join(codes, " "), "rate_limit") || rateLimitPattern.MatchString(text):
		set(ErrorRateLimit, "provider rate limit exceeded", true)
	case status == 408 || status == 504 || has("deadline_exceeded"):
		set(ErrorTimeout, "provider response timed out", true)
	case status == 409 || status >= 500 || status < 400 &&
		((!has("invalid_prompt", "invalid_request_error", "validationexception") && serverPattern.MatchString(text)) ||
			has("api_error", "internal_error", "internalserverexception", "modelstreamerrorexception", "overloaded_error", "server_error", "server_is_overloaded", "slow_down", "serviceunavailableexception", "internal", "unavailable") ||
			strings.Contains(strings.Join(codes, " "), "exhausted") || strings.Contains(strings.Join(codes, " "), "unavailable")):
		set(ErrorUnavailable, "provider is unavailable", true)
	case status >= 400 && status < 500 || has("invalid_prompt", "invalid_request_error", "validationexception", "invalid_argument", "not_found", "not_found_error", "failed_precondition"):
		set(ErrorInvalidRequest, "provider rejected the request", false)
	}
	return failure
}

func providerFailureDetails(body string) (*CallDiagnostic, []string) {
	d := &CallDiagnostic{Body: body}
	var root map[string]json.RawMessage
	if json.Unmarshal([]byte(body), &root) != nil {
		return d, nil
	}
	object := func(parent map[string]json.RawMessage, key string) map[string]json.RawMessage {
		var value map[string]json.RawMessage
		_ = json.Unmarshal(parent[key], &value)
		return value
	}
	field := func(parent map[string]json.RawMessage, key string) string {
		var text string
		if json.Unmarshal(parent[key], &text) == nil {
			return text
		}
		var number json.Number
		if json.Unmarshal(parent[key], &number) == nil {
			return number.String()
		}
		return ""
	}
	errorObject := object(root, "error")
	response := object(root, "response")
	responseError := object(response, "error")
	inner := object(errorObject, "innererror")
	metadata := object(errorObject, "metadata")
	exception := object(root, "exception")
	d.Event = field(root, "type")
	// Prefer the native error object, with top-level fallbacks for mixed envelopes.
	for _, detail := range []map[string]json.RawMessage{responseError, errorObject, exception, root} {
		if d.Code == "" {
			d.Code = field(detail, "code")
		}
		if d.Type == "" {
			d.Type = field(detail, "type")
			if d.Type == "error" || strings.HasPrefix(d.Type, "response.") {
				d.Type = ""
			}
			if d.Type == "" {
				d.Type = field(detail, "status")
			}
		}
		if d.Param == "" {
			d.Param = field(detail, "param")
		}
		if d.Message == "" {
			d.Message = field(detail, "message")
		}
	}
	var codes []string
	for _, detail := range []map[string]json.RawMessage{root, errorObject, inner, metadata, responseError, response, exception} {
		for _, key := range []string{"code", "type", "status", "error_type"} {
			if value := field(detail, key); value != "" {
				codes = append(codes, strings.ToLower(value))
			}
		}
	}
	if match := gatewayCodePattern.FindStringSubmatch(d.Message); len(match) > 1 {
		codes = append(codes, strings.ToLower(match[1]))
	}
	return d, codes
}
