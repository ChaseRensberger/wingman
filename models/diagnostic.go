package models

import (
	"runtime"
	"runtime/debug"
)

// CallDiagnostic records bounded failure evidence with known credentials redacted.
type CallDiagnostic struct {
	Stage           string               `json:"stage"`
	Route           string               `json:"route,omitempty"`
	Protocol        string               `json:"protocol,omitempty"`
	Endpoint        string               `json:"endpoint,omitempty"`
	Model           string               `json:"model,omitempty"`
	Settings        map[string]any       `json:"settings,omitempty"`
	HTTPStatus      int                  `json:"http_status,omitempty"`
	RequestID       string               `json:"request_id,omitempty"`
	Category        ErrorCategory        `json:"category"`
	Classification  string               `json:"classification,omitempty"`
	Retryable       bool                 `json:"retryable"`
	RetryAfterMS    int64                `json:"retry_after_ms,omitempty"`
	Event           string               `json:"event,omitempty"`
	Code            string               `json:"code,omitempty"`
	Type            string               `json:"type,omitempty"`
	Param           string               `json:"param,omitempty"`
	Message         string               `json:"message,omitempty"`
	Body            string               `json:"body,omitempty"`
	BodyKind        string               `json:"body_kind,omitempty"`
	ResponseHeaders map[string][]string  `json:"response_headers,omitempty"`
	RateLimit       *RateLimitDiagnostic `json:"rate_limit,omitempty"`
	Transport       *TransportDiagnostic `json:"transport,omitempty"`
	Causes          []DiagnosticCause    `json:"causes,omitempty"`
	OutputStarted   bool                 `json:"output_started"`
	Redacted        bool                 `json:"redacted,omitempty"`
	Truncated       bool                 `json:"truncated,omitempty"`
	DetailOmitted   bool                 `json:"detail_omitted,omitempty"`
}

// CallRetry records the decision made after a failed physical attempt.
type CallRetry struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
	DelayMS  *int64 `json:"delay_ms,omitempty"`
}

// CallTiming records elapsed milliseconds from the start of one physical attempt.
type CallTiming struct {
	FirstResponseMS *int64 `json:"first_response_ms,omitempty"`
	FirstActivityMS *int64 `json:"first_activity_ms,omitempty"`
	FirstAnswerMS   *int64 `json:"first_answer_ms,omitempty"`
}

// RateLimitDiagnostic records provider limits, remaining capacity, and reset times.
type RateLimitDiagnostic struct {
	Limit     map[string]string `json:"limit,omitempty"`
	Remaining map[string]string `json:"remaining,omitempty"`
	Reset     map[string]string `json:"reset,omitempty"`
}

// TransportDiagnostic identifies the operation and known delivery state of a failure.
type TransportDiagnostic struct {
	Kind      string `json:"kind"`
	Operation string `json:"operation"`
	Code      string `json:"code,omitempty"`
	Phase     string `json:"phase,omitempty"`
	Delivery  string `json:"delivery"`
	Recovery  string `json:"recovery"`
}

// DiagnosticCause preserves a native error and its wrapped or joined causes.
type DiagnosticCause struct {
	Type    string            `json:"type"`
	Message string            `json:"message"`
	Code    string            `json:"code,omitempty"`
	Stack   string            `json:"stack,omitempty"`
	Causes  []DiagnosticCause `json:"causes,omitempty"`
}

// BuildTrace identifies the executable that captured a model call.
type BuildTrace struct {
	Version  string `json:"version"`
	Revision string `json:"revision,omitempty"`
	Modified bool   `json:"modified,omitempty"`
	Go       string `json:"go"`
}

var callBuild = func() BuildTrace {
	build := BuildTrace{Version: "unknown", Go: runtime.Version()}
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Path == "github.com/chaserensberger/wingman" {
			build.Version = info.Main.Version
		} else {
			for _, dep := range info.Deps {
				if dep.Path == "github.com/chaserensberger/wingman" {
					build.Version = dep.Version
				}
			}
		}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				build.Revision = setting.Value
			case "vcs.modified":
				build.Modified = setting.Value == "true"
			}
		}
	}
	return build
}()
