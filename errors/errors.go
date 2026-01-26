package errors

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	ErrProviderUnavailable ErrorCode = "provider_unavailable"
	ErrRateLimit           ErrorCode = "rate_limit"
	ErrContextTooLong      ErrorCode = "context_too_long"
	ErrToolFailed          ErrorCode = "tool_failed"
	ErrToolNotFound        ErrorCode = "tool_not_found"
	ErrPermissionDenied    ErrorCode = "permission_denied"
	ErrTimeout             ErrorCode = "timeout"
	ErrInvalidInput        ErrorCode = "invalid_input"
	ErrMaxStepsExceeded    ErrorCode = "max_steps_exceeded"
	ErrStreamingFailed     ErrorCode = "streaming_failed"
)

type WingmanError struct {
	Code      ErrorCode
	Message   string
	Cause     error
	Retryable bool
}

func (e *WingmanError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *WingmanError) Unwrap() error {
	return e.Cause
}

func New(code ErrorCode, message string) *WingmanError {
	return &WingmanError{
		Code:      code,
		Message:   message,
		Retryable: isRetryable(code),
	}
}

func Wrap(code ErrorCode, message string, cause error) *WingmanError {
	return &WingmanError{
		Code:      code,
		Message:   message,
		Cause:     cause,
		Retryable: isRetryable(code),
	}
}

func isRetryable(code ErrorCode) bool {
	switch code {
	case ErrRateLimit, ErrProviderUnavailable, ErrTimeout:
		return true
	default:
		return false
	}
}

func IsRetryable(err error) bool {
	var we *WingmanError
	if errors.As(err, &we) {
		return we.Retryable
	}
	return false
}

func GetCode(err error) ErrorCode {
	var we *WingmanError
	if errors.As(err, &we) {
		return we.Code
	}
	return ""
}

func Is(err error, code ErrorCode) bool {
	return GetCode(err) == code
}

func ProviderUnavailable(message string, cause error) *WingmanError {
	return Wrap(ErrProviderUnavailable, message, cause)
}

func RateLimit(message string, cause error) *WingmanError {
	return Wrap(ErrRateLimit, message, cause)
}

func ContextTooLong(message string) *WingmanError {
	return New(ErrContextTooLong, message)
}

func ToolFailed(toolName string, cause error) *WingmanError {
	return Wrap(ErrToolFailed, fmt.Sprintf("tool '%s' failed", toolName), cause)
}

func ToolNotFound(toolName string) *WingmanError {
	return New(ErrToolNotFound, fmt.Sprintf("tool '%s' not found", toolName))
}

func PermissionDenied(action string) *WingmanError {
	return New(ErrPermissionDenied, fmt.Sprintf("permission denied: %s", action))
}

func Timeout(operation string, cause error) *WingmanError {
	return Wrap(ErrTimeout, fmt.Sprintf("timeout during %s", operation), cause)
}

func InvalidInput(message string) *WingmanError {
	return New(ErrInvalidInput, message)
}

func MaxStepsExceeded(steps int) *WingmanError {
	return New(ErrMaxStepsExceeded, fmt.Sprintf("exceeded maximum steps (%d)", steps))
}

func StreamingFailed(cause error) *WingmanError {
	return Wrap(ErrStreamingFailed, "streaming failed", cause)
}
