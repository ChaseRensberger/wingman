package route

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/chaserensberger/wingman/models"
)

const diagnosticBodyLimit = 64 << 10
const diagnosticFieldLimit = 2048

var diagnosticCredential = regexp.MustCompile(`(?i)\b(?:Bearer\s+[^\s"'\\,;<>]+|sk-[a-z0-9_-]+|AIza[a-z0-9_-]+)`)

func captureDiagnostic(err error, prepared *models.PreparedRequest, request *http.Request, headers http.Header, stage, data string, output bool) {
	var failure *models.ProviderError
	if !errors.As(err, &failure) {
		return
	}
	d := failure.Diagnostic
	if d == nil {
		d = models.ClassifyProviderFailure(failure.Provider, failure.Status, data).Diagnostic
		failure.Diagnostic = d
	}
	d.Stage, d.OutputStarted = stage, output
	d.Classification = failure.Classification
	d.Category, d.HTTPStatus, d.RequestID, d.Retryable = failure.Category, failure.Status, failure.RequestID, failure.Retryable
	if failure.RetryAfter != nil {
		d.RetryAfterMS = failure.RetryAfter.Milliseconds()
	}
	if prepared != nil {
		d.Route, _ = prepared.Metadata["route"].(string)
		d.Protocol, _ = prepared.Metadata["protocol"].(string)
		d.Model, _ = prepared.Body["model"].(string)
		if d.Model == "" {
			d.Model = prepared.Model.ID
		}
		endpoint, parseErr := url.Parse(prepared.URL)
		if parseErr == nil {
			endpoint.User, endpoint.RawQuery, endpoint.Fragment, endpoint.RawFragment = nil, "", "", ""
			endpoint.ForceQuery = false
			d.Endpoint = endpoint.String()
		}
		d.Settings = diagnosticSettings(prepared.Body)
	}
	if d.Body == "" {
		d.Body = data
	}
	if d.Body != "" && d.BodyKind == "" {
		d.BodyKind = "response"
		if stage == "stream" {
			d.BodyKind = "event"
		}
	}
	sanitize := diagnosticCredentialSanitizer(prepared, request, headers)
	clean := func(value string, limit int) string {
		result := sanitize(value, len(value) > limit)
		d.Redacted = d.Redacted || result != value
		if len(value) > limit || len(result) > limit {
			d.Truncated = true
		}
		if len(result) > limit {
			result = result[:limit]
			for !utf8.ValidString(result) {
				result = result[:len(result)-1]
			}
		}
		return result
	}
	for _, field := range []*string{&d.Route, &d.Protocol, &d.Endpoint, &d.Model, &d.RequestID, &d.Event, &d.Code, &d.Type, &d.Param, &d.Message} {
		*field = clean(*field, diagnosticFieldLimit)
	}
	cause := failure.Cause
	// Protocol errors wrap the original event as a cause. The body already retains it.
	if cause != nil && cause.Error() == d.Body {
		cause = nil
	}
	d.Body = clean(d.Body, diagnosticBodyLimit)
	d.Transport = diagnosticTransport(failure, stage)
	d.Causes = diagnosticCauses(cause, clean, &d.Truncated)
	var names []string
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	d.ResponseHeaders = map[string][]string{}
	// Bound the total header bytes as well as each value and the number of names.
	budget := 16 << 10
	for i, name := range names {
		if i >= 64 || budget <= 0 {
			d.Truncated = true
			break
		}
		key := clean(strings.ToLower(name), min(budget, 256))
		budget -= len(key)
		if sensitiveDiagnosticHeader(name) {
			if budget < len("[redacted]") {
				d.Truncated = true
				break
			}
			d.ResponseHeaders[key], d.Redacted = []string{"[redacted]"}, true
			budget -= len("[redacted]")
			continue
		}
		for _, value := range headers[name] {
			if budget <= 0 {
				d.Truncated = true
				break
			}
			value = clean(value, min(budget, diagnosticFieldLimit))
			budget -= max(1, len(value))
			d.ResponseHeaders[key] = append(d.ResponseHeaders[key], value)
		}
	}
	d.RateLimit = diagnosticRateLimit(d.ResponseHeaders)
}

func diagnosticSettings(body map[string]any) map[string]any {
	settings := map[string]any{}
	for _, key := range []string{"temperature", "top_p", "max_tokens", "max_output_tokens", "max_completion_tokens", "stream", "parallel_tool_calls"} {
		switch value := body[key].(type) {
		case bool, int, int64:
			settings[key] = value
		case float64:
			if !math.IsNaN(value) && !math.IsInf(value, 0) {
				settings[key] = value
			}
		case json.Number:
			if n, err := value.Float64(); err == nil && !math.IsNaN(n) && !math.IsInf(n, 0) {
				settings[key] = value
			}
		}
	}
	// String settings use known enums; arbitrary overlay values can contain content.
	for _, key := range []string{"reasoning_effort", "tool_choice"} {
		if value, ok := body[key].(string); ok {
			switch value {
			case "none", "minimal", "low", "medium", "high", "xhigh", "auto", "required":
				settings[key] = value
			}
		}
	}
	for _, key := range []string{"reasoning", "generationConfig"} {
		if nested, ok := body[key].(map[string]any); ok {
			for name, value := range nested {
				switch name {
				case "effort", "summary":
					switch value {
					case "none", "minimal", "low", "medium", "high", "xhigh", "auto", "concise", "detailed":
						settings[key+"."+name] = value
					}
				case "temperature", "topP", "topK", "maxOutputTokens":
					switch value.(type) {
					case int:
						settings[key+"."+name] = value
					case float64:
						if n := value.(float64); !math.IsNaN(n) && !math.IsInf(n, 0) {
							settings[key+"."+name] = value
						}
					}
				}
			}
		}
	}
	return settings
}

func diagnosticCredentialSanitizer(prepared *models.PreparedRequest, request *http.Request, headers http.Header) func(string, bool) string {
	secrets := map[string]bool{}
	add := func(value string) {
		if value != "" {
			secrets[value] = true
			encoded, _ := json.Marshal(value)
			secrets[string(encoded[1:len(encoded)-1])] = true
			secrets[url.QueryEscape(value)] = true
			secrets[url.PathEscape(value)] = true
		}
	}
	addHeader := func(name, value string) {
		switch strings.ToLower(name) {
		case "accept", "content-type", "content-length", "user-agent", "accept-encoding":
			return
		}
		add(value)
		if strings.EqualFold(name, "authorization") || strings.EqualFold(name, "proxy-authorization") {
			if parts := strings.Fields(value); len(parts) > 1 {
				for _, part := range parts[1:] {
					add(part)
				}
			}
		}
	}
	var urls []string
	if prepared != nil {
		for name, value := range prepared.Headers {
			addHeader(name, value)
		}
		urls = append(urls, prepared.URL)
	}
	for name, values := range headers {
		if sensitiveDiagnosticHeader(name) {
			for _, value := range values {
				addHeader(name, value)
			}
		}
	}
	for _, cookie := range (&http.Response{Header: headers}).Cookies() {
		add(cookie.Value)
	}
	if request != nil {
		urls = append(urls, request.URL.String())
		for name, values := range request.Header {
			for _, value := range values {
				addHeader(name, value)
			}
		}
		if user, password, ok := request.BasicAuth(); ok {
			add(user)
			add(password)
		}
		for _, cookie := range request.Cookies() {
			add(cookie.Value)
		}
	}
	for _, raw := range urls {
		if parsed, err := url.Parse(raw); err == nil {
			if parsed.User != nil {
				add(parsed.User.Username())
				password, _ := parsed.User.Password()
				add(password)
			}
			for _, values := range parsed.Query() {
				for _, value := range values {
					add(value)
				}
			}
		}
	}
	values := make([]string, 0, len(secrets))
	for value := range secrets {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	pairs := make([]string, 0, len(values)*2)
	for _, value := range values {
		pairs = append(pairs, value, "[redacted]")
	}
	replacer := strings.NewReplacer(pairs...)
	return func(value string, truncated bool) string {
		// A bounded HTTP read can end in the middle of a known credential.
		if truncated {
			for _, secret := range values {
				for i := max(0, len(value)-len(secret)+1); i < len(value); i++ {
					if strings.HasPrefix(secret, value[i:]) {
						value = value[:i] + "[redacted]"
						break
					}
				}
			}
		}
		return diagnosticCredential.ReplaceAllString(replacer.Replace(value), "[redacted]")
	}
}

func sensitiveDiagnosticHeader(name string) bool {
	name = strings.ToLower(name)
	for _, part := range strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' }) {
		switch part {
		case "authorization", "cookie", "token", "secret", "key", "apikey":
			return true
		}
	}
	return false
}

func diagnosticRateLimit(headers map[string][]string) *models.RateLimitDiagnostic {
	d := &models.RateLimitDiagnostic{Limit: map[string]string{}, Remaining: map[string]string{}, Reset: map[string]string{}}
	for name, values := range headers {
		for kind, target := range map[string]map[string]string{"limit": d.Limit, "remaining": d.Remaining, "reset": d.Reset} {
			key := strings.TrimPrefix(name, "x-ratelimit-"+kind+"-")
			if key == name && strings.HasPrefix(name, "anthropic-ratelimit-") && strings.HasSuffix(name, "-"+kind) {
				key = strings.TrimSuffix(strings.TrimPrefix(name, "anthropic-ratelimit-"), "-"+kind)
			}
			if key != name && key != "" {
				target[key] = strings.Join(values, ", ")
			}
		}
	}
	if len(d.Limit)+len(d.Remaining)+len(d.Reset) == 0 {
		return nil
	}
	return d
}

func diagnosticTransport(failure *models.ProviderError, stage string) *models.TransportDiagnostic {
	d := &models.TransportDiagnostic{Kind: "http", Operation: "request", Delivery: "ambiguous", Recovery: "fail"}
	switch stage {
	case "prepare":
		d.Phase, d.Delivery = "prepare", "not-sent"
	case "response":
		d.Operation, d.Phase, d.Delivery = "read", "receive", "rejected"
	case "stream":
		d.Operation, d.Phase, d.Delivery = "read", "receive", "accepted"
		if failure.Category == models.ErrorDecoding {
			d.Phase = "decode"
		}
	case "request":
		var op *net.OpError
		if errors.As(failure.Cause, &op) && op.Op == "dial" {
			d.Phase, d.Delivery = "connect", "not-sent"
		}
	}
	if failure.Retryable && (stage == "request" || stage == "response") {
		d.Recovery = "retry-full"
	}
	var errno syscall.Errno
	var dns *net.DNSError
	switch {
	case errors.As(failure.Cause, &errno):
		d.Code = diagnosticNativeCode(errno)
	case errors.As(failure.Cause, &dns):
		if dns.IsNotFound {
			d.Code = "DNS_NOT_FOUND"
		} else if dns.IsTimeout {
			d.Code = "DNS_TIMEOUT"
		}
	case failure.Category == models.ErrorTimeout:
		d.Code = "TIMEOUT"
	case failure.Category == models.ErrorCancellation:
		d.Code = "CANCELED"
	case errors.Is(failure.Cause, io.ErrUnexpectedEOF):
		d.Code = "UNEXPECTED_EOF"
	}
	return d
}

func diagnosticNativeCode(err error) string {
	switch err {
	case syscall.ECONNREFUSED:
		return "ECONNREFUSED"
	case syscall.ECONNRESET:
		return "ECONNRESET"
	case syscall.ECONNABORTED:
		return "ECONNABORTED"
	case syscall.ETIMEDOUT:
		return "ETIMEDOUT"
	case syscall.EPIPE:
		return "EPIPE"
	case syscall.ENETUNREACH:
		return "ENETUNREACH"
	case syscall.EHOSTUNREACH:
		return "EHOSTUNREACH"
	}
	if errno, ok := err.(syscall.Errno); ok {
		return strconv.FormatUint(uint64(errno), 10)
	}
	return ""
}

func diagnosticCauses(err error, clean func(string, int) string, truncated *bool) []models.DiagnosticCause {
	count := 0
	var visit func(error, int) []models.DiagnosticCause
	visit = func(err error, depth int) []models.DiagnosticCause {
		if err == nil {
			return nil
		}
		if depth == 8 || count == 16 {
			*truncated = true
			return nil
		}
		count++
		message := err.Error()
		d := models.DiagnosticCause{Type: clean(fmt.Sprintf("%T", err), 256), Message: clean(message, diagnosticFieldLimit), Code: diagnosticNativeCode(err)}
		if _, ok := err.(fmt.Formatter); ok {
			if stack := fmt.Sprintf("%+v", err); stack != message {
				d.Stack = clean(stack, 8<<10)
			}
		}
		switch wrapped := err.(type) {
		case interface{ Unwrap() []error }:
			for _, child := range wrapped.Unwrap() {
				if count == 16 {
					*truncated = true
					break
				}
				d.Causes = append(d.Causes, visit(child, depth+1)...)
			}
		case interface{ Unwrap() error }:
			d.Causes = visit(wrapped.Unwrap(), depth+1)
		}
		return []models.DiagnosticCause{d}
	}
	return visit(err, 0)
}

type diagnosticReader struct {
	io.Reader
	prefix []byte
}

func (r *diagnosticReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.prefix = append(r.prefix, p[:min(n, diagnosticBodyLimit+1-len(r.prefix))]...)
	return n, err
}
