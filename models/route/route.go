package route

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/chaserensberger/wingman/models"
)

// Endpoint identifies a deployment without interpreting its protocol.
type Endpoint struct {
	BaseURL string
	Path    string
	Query   map[string]string
}

// Route composes a protocol with its deployment and transport.
type Route struct {
	ID        string
	Protocol  Protocol
	Endpoint  Endpoint
	Auth      Auth
	Headers   map[string]string
	Transport Transport
	Framing   Framing
	// Body applies deployment requirements after request overrides.
	Body func(map[string]any) map[string]any
}

// URL resolves the endpoint with final request query overrides.
func (r Route) URL(query map[string]string) string {
	raw := strings.TrimRight(r.Endpoint.BaseURL, "/") + r.Endpoint.Path
	values := url.Values{}
	for k, v := range r.Endpoint.Query {
		values.Set(k, v)
	}
	for k, v := range query {
		values.Set(k, v)
	}
	if len(values) > 0 {
		raw += "?" + values.Encode()
	}
	return raw
}

// Apply adds deployment headers and authentication to an outbound request.
func (r Route) Apply(req *http.Request) error {
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	if r.Auth != nil {
		return r.Auth.Apply(req)
	}
	return nil
}

// Model binds catalog metadata and a variant to an executable route.
type Model struct {
	Info_   models.ModelInfo
	Variant models.ModelVariant
	Route   Route
}
