package route

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chaserensberger/wingman/models"
)

func responseError(provider string, resp *http.Response, body string) *models.ProviderError {
	failure := models.ClassifyProviderFailure(provider, resp.StatusCode, body)
	failure.RequestID, failure.RetryAfter = responseRequestID(resp.Header), retryAfter(resp.Header)
	return failure
}

func transportError(provider string, err error) *models.ProviderError {
	category := models.ErrorTransport
	retryable := true
	switch {
	case errors.Is(err, context.Canceled):
		category, retryable = models.ErrorCancellation, false
	case errors.Is(err, context.DeadlineExceeded):
		category = models.ErrorTimeout
	default:
		var networkErr net.Error
		if errors.As(err, &networkErr) && networkErr.Timeout() {
			category = models.ErrorTimeout
		}
	}
	return &models.ProviderError{Category: category, Provider: provider, Retryable: retryable, Message: "provider transport failed", Cause: err}
}

func decodingError(provider, message string, cause error) *models.ProviderError {
	return &models.ProviderError{Category: models.ErrorDecoding, Provider: provider, Message: message, Cause: cause}
}

func retryAfter(headers http.Header) *time.Duration {
	if ms, err := strconv.ParseFloat(headers.Get("Retry-After-Ms"), 64); err == nil && ms >= 0 && ms < float64(1<<63)/float64(time.Millisecond) {
		d := time.Duration(ms * float64(time.Millisecond))
		return &d
	}
	raw := strings.TrimSpace(headers.Get("Retry-After"))
	if raw == "" {
		return nil
	}
	if seconds, err := strconv.ParseFloat(raw, 64); err == nil && seconds >= 0 && seconds < float64(1<<63)/float64(time.Second) {
		d := time.Duration(seconds * float64(time.Second))
		return &d
	}
	if when, err := http.ParseTime(raw); err == nil {
		d := time.Until(when)
		if d < 0 {
			d = 0
		}
		return &d
	}
	return nil
}
