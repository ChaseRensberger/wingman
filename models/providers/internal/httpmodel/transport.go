package httpmodel

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/chaserensberger/wingman/models"
)

type streamTransport struct {
	client *http.Client
}

func (t streamTransport) open(ctx context.Context, provider string, route Route, headers map[string]string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, route.URL(), bytes.NewReader(body))
	if err != nil {
		return nil, &models.ProviderError{Category: models.ErrorInvalidRequest, Provider: provider, Message: "invalid provider request", Cause: err}
	}
	req.Header.Set("content-type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	if err := route.Apply(req); err != nil {
		return nil, &models.ProviderError{Category: models.ErrorAuthentication, Provider: provider, Message: "provider authentication failed", Cause: err}
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, transportError(provider, err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
	return nil, responseError(provider, resp)
}
