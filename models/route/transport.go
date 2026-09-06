package route

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/chaserensberger/wingman/models"
)

// Transport opens a response for a prepared request.
type Transport interface {
	Open(context.Context, Route, *models.PreparedRequest) (*http.Response, error)
}

// HTTP sends prepared JSON requests through an HTTP client.
type HTTP struct{ Client *http.Client }

// Open sends one physical request and checks its HTTP status.
func (t HTTP) Open(ctx context.Context, deployment Route, prepared *models.PreparedRequest) (*http.Response, error) {
	body, err := json.Marshal(prepared.Body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, prepared.URL, bytes.NewReader(body))
	if err != nil {
		return nil, &models.ProviderError{Category: models.ErrorInvalidRequest, Provider: prepared.Model.Provider, Message: "invalid provider request", Cause: err}
	}
	for k, v := range prepared.Headers {
		req.Header.Set(k, v)
	}
	if err := deployment.Apply(req); err != nil {
		return nil, &models.ProviderError{Category: models.ErrorAuthentication, Provider: prepared.Model.Provider, Message: "provider authentication failed", Cause: err}
	}
	client := t.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, transportError(prepared.Model.Provider, err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
	return nil, responseError(prepared.Model.Provider, resp)
}
