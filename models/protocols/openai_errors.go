package protocols

import (
	"encoding/json"
	"errors"

	"github.com/chaserensberger/wingman/models"
)

type openAIError struct {
	Code openAIErrorCode `json:"code"`
	Type string          `json:"type"`
}

type openAIErrorCode string

func (c *openAIErrorCode) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		*c = openAIErrorCode(text)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return err
	}
	*c = openAIErrorCode(number.String())
	return nil
}

func openAIFailure(provider, code string, nested *openAIError, data string) *models.ProviderError {
	if code == "" && nested != nil {
		code = string(nested.Code)
		if code == "" {
			code = nested.Type
		}
	}
	// Native messages can contain request data; only the internal cause retains the frame.
	failure := &models.ProviderError{Provider: provider, Category: models.ErrorProvider, Message: "provider response failed", Cause: errors.New(data)}
	switch code {
	case "401", "invalid_api_key", "authentication_error":
		failure.Category, failure.Message = models.ErrorAuthentication, "provider authentication failed"
	case "403", "permission_denied", "authorization_error":
		failure.Category, failure.Message = models.ErrorAuthorization, "provider access denied"
	case "429", "rate_limit_exceeded", "rate_limit_error":
		failure.Category, failure.Message, failure.Retryable = models.ErrorRateLimit, "provider rate limit exceeded", true
	case "402", "insufficient_quota":
		failure.Category, failure.Message = models.ErrorRateLimit, "provider quota exceeded"
	case "400", "404", "422", "invalid_prompt", "invalid_request_error", "context_length_exceeded":
		failure.Category, failure.Message = models.ErrorInvalidRequest, "provider rejected the request"
	case "408", "504":
		failure.Category, failure.Message, failure.Retryable = models.ErrorTimeout, "provider response timed out", true
	case "500", "502", "503", "server_error", "internal_error":
		failure.Category, failure.Message, failure.Retryable = models.ErrorUnavailable, "provider is unavailable", true
	}
	return failure
}
