package api

// ErrorCode is a stable machine-readable HTTP failure category.
type ErrorCode string

const (
	ErrorCodeInvalidRequest   ErrorCode = "invalid_request"
	ErrorCodeUnauthorized     ErrorCode = "unauthorized"
	ErrorCodeForbidden        ErrorCode = "forbidden"
	ErrorCodeNotFound         ErrorCode = "not_found"
	ErrorCodeMethodNotAllowed ErrorCode = "method_not_allowed"
	ErrorCodeConflict         ErrorCode = "conflict"
	ErrorCodePayloadTooLarge  ErrorCode = "payload_too_large"
	ErrorCodeUnsupportedMedia ErrorCode = "unsupported_media_type"
	ErrorCodeValidationFailed ErrorCode = "validation_failed"
	ErrorCodeRateLimited      ErrorCode = "rate_limited"
	ErrorCodeInternal         ErrorCode = "internal_error"
	ErrorCodeNotImplemented   ErrorCode = "not_implemented"
	ErrorCodeUpstream         ErrorCode = "upstream_error"
	ErrorCodeUnavailable      ErrorCode = "unavailable"
	ErrorCodeTimeout          ErrorCode = "timeout"
	ErrorCodeRunFailed        ErrorCode = "run_failed"
	ErrorCodeRequestFailed    ErrorCode = "request_failed"
)

// Error describes one public API failure.
type Error struct {
	Code      ErrorCode     `json:"code"`
	Message   string        `json:"message"`
	RequestID string        `json:"request_id,omitempty"`
	Details   []ErrorDetail `json:"details,omitempty"`
}

// ErrorDetail identifies one invalid request field.
type ErrorDetail struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

// ErrorResponse is the canonical non-success HTTP response body.
type ErrorResponse struct {
	Error Error `json:"error"`
}
