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
		failure := &models.ProviderError{Category: models.ErrorInvalidRequest, Provider: prepared.Model.Provider, Message: "invalid provider request", Cause: err}
		captureDiagnostic(failure, prepared, nil, nil, "prepare", "", false)
		return nil, failure
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, prepared.URL, bytes.NewReader(body))
	if err != nil {
		failure := &models.ProviderError{Category: models.ErrorInvalidRequest, Provider: prepared.Model.Provider, Message: "invalid provider request", Cause: err}
		captureDiagnostic(failure, prepared, nil, nil, "prepare", "", false)
		return nil, failure
	}
	for k, v := range prepared.Headers {
		req.Header.Set(k, v)
	}
	if err := deployment.Apply(req); err != nil {
		failure := &models.ProviderError{Category: models.ErrorAuthentication, Provider: prepared.Model.Provider, Message: "provider authentication failed", Cause: err}
		captureDiagnostic(failure, prepared, req, nil, "prepare", "", false)
		return nil, failure
	}
	client := t.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		failure := transportError(prepared.Model.Provider, err)
		captureDiagnostic(failure, prepared, req, nil, "request", "", false)
		return nil, failure
	}
	if resp.Request == nil {
		resp.Request = req
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, diagnosticBodyLimit+1))
	_ = resp.Body.Close()
	failure := responseError(prepared.Model.Provider, resp, string(body))
	failure.Cause = readErr
	captureDiagnostic(failure, prepared, req, resp.Header, "response", string(body), false)
	if readErr != nil {
		failure.Diagnostic.DetailOmitted = true
	}
	return nil, failure
}
