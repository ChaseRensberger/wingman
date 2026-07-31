package models

import (
	"fmt"
	"time"
)

// ErrorCategory classifies a provider-neutral model failure.
type ErrorCategory string

const (
	ErrorAuthentication ErrorCategory = "authentication"
	ErrorAuthorization  ErrorCategory = "authorization"
	ErrorRateLimit      ErrorCategory = "rate_limit"
	ErrorInvalidRequest ErrorCategory = "invalid_request"
	ErrorUnavailable    ErrorCategory = "unavailable"
	ErrorTimeout        ErrorCategory = "timeout"
	ErrorTransport      ErrorCategory = "transport"
	ErrorProvider       ErrorCategory = "provider"
	ErrorDecoding       ErrorCategory = "decoding"
	ErrorCancellation   ErrorCategory = "cancellation"
)

// ProviderError is a safe, provider-neutral failure returned by model clients.
// Its cause remains available through errors.Is and errors.As.
type ProviderError struct {
	Category   ErrorCategory
	Provider   string
	Status     int
	RequestID  string
	Retryable  bool
	RetryAfter *time.Duration
	Message    string
	Metadata   map[string]string
	Cause      error
}

// Error returns a safe diagnostic and never includes a provider response body.
func (e *ProviderError) Error() string {
	if e == nil {
		return "<nil>"
	}
	message := e.Message
	if message == "" {
		message = string(e.Category)
	}
	if e.Provider == "" {
		return message
	}
	if e.Status != 0 {
		return fmt.Sprintf("%s: %s (HTTP %d)", e.Provider, message, e.Status)
	}
	return fmt.Sprintf("%s: %s", e.Provider, message)
}

// Unwrap returns the underlying provider or transport error.
func (e *ProviderError) Unwrap() error { return e.Cause }

// ProviderRequestID returns the provider request ID when one was supplied.
func (e *ProviderError) ProviderRequestID() string { return e.RequestID }
