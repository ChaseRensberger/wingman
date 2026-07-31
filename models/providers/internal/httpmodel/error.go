package httpmodel

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

func responseError(provider string, resp *http.Response) *models.ProviderError {
	category := models.ErrorProvider
	retryable := false
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		category = models.ErrorAuthentication
	case resp.StatusCode == http.StatusForbidden:
		category = models.ErrorAuthorization
	case resp.StatusCode == http.StatusRequestTimeout:
		category, retryable = models.ErrorTimeout, true
	case resp.StatusCode == http.StatusTooManyRequests:
		category, retryable = models.ErrorRateLimit, true
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		category = models.ErrorInvalidRequest
	case resp.StatusCode >= 500:
		category, retryable = models.ErrorUnavailable, true
	}
	return &models.ProviderError{Category: category, Provider: provider, Status: resp.StatusCode, RequestID: responseRequestID(resp.Header), Retryable: retryable, RetryAfter: retryAfter(resp.Header), Message: "provider request failed"}
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

func isTransportError(err error) bool {
	var networkErr net.Error
	return errors.As(err, &networkErr)
}

func retryAfter(headers http.Header) *time.Duration {
	raw := strings.TrimSpace(headers.Get("Retry-After"))
	if raw == "" {
		return nil
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
		d := time.Duration(seconds) * time.Second
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
