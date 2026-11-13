package pluginrpc

import (
	"time"

	"github.com/chaserensberger/wingman/models"
)

// Error is a JSON-RPC fault with optional safe provider failure details.
type Error struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Data    *providerFailure `json:"data,omitempty"`
}

type providerFailure struct {
	Category       models.ErrorCategory `json:"category"`
	Classification string               `json:"classification,omitempty"`
	Provider       string               `json:"provider,omitempty"`
	Status         int                  `json:"status,omitempty"`
	RequestID      string               `json:"request_id,omitempty"`
	Retryable      bool                 `json:"retryable,omitempty"`
	RetryAfter     *time.Duration       `json:"retry_after,omitempty"`
	Message        string               `json:"message"`
	Metadata       map[string]string    `json:"metadata,omitempty"`
}

// NewError preserves provider failure details without transmitting causes or diagnostics.
func NewError(err error) *Error {
	fault := &Error{Code: -32000, Message: err.Error()}
	for cause := err; cause != nil; {
		if failure, ok := cause.(*models.ProviderError); ok {
			fault.Data = &providerFailure{
				Category: failure.Category, Classification: failure.Classification,
				Provider: failure.Provider, Status: failure.Status, RequestID: failure.RequestID,
				Retryable: failure.Retryable, RetryAfter: failure.RetryAfter,
				Message: failure.Message, Metadata: failure.Metadata,
			}
			break
		}
		// A joined failure can include a persistence error that forbids retry.
		wrapped, ok := cause.(interface{ Unwrap() error })
		if !ok {
			break
		}
		cause = wrapped.Unwrap()
	}
	return fault
}

// Error returns the remote failure message.
func (e *Error) Error() string { return e.Message }

// Unwrap restores the provider-neutral failure for runtime classification.
func (e *Error) Unwrap() error {
	if e.Data == nil {
		return nil
	}
	failure := e.Data
	return &models.ProviderError{
		Category: failure.Category, Classification: failure.Classification,
		Provider: failure.Provider, Status: failure.Status, RequestID: failure.RequestID,
		Retryable: failure.Retryable, RetryAfter: failure.RetryAfter,
		Message: failure.Message, Metadata: failure.Metadata,
	}
}
