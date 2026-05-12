// Package route provides the composable model-routing layer used by provider
// implementations. A Route combines a protocol adapter with endpoint, auth,
// and transport concerns while still presenting the existing models.Model
// interface to callers.
package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/chaserensberger/wingman/models"
)

// ModelRef describes one provider/model/deployment selection as data.
// Provider facades construct these and Routes execute them.
type ModelRef struct {
	Provider        string
	ModelID         string
	BaseURL         string
	MaxOutputTokens int
	Info            models.ModelInfo
	Compat          any
	Headers         http.Header
}

// PreparedBody is the protocol-owned request payload plus protocol-specific
// headers that should be applied before auth.
type PreparedBody struct {
	Body    []byte
	Headers http.Header
	Meta    map[string]any
}

// PreparedRequest is the transport-ready HTTP request data. It is useful for
// tests and request inspection because preparing does not perform I/O.
type PreparedRequest struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
	Meta    map[string]any
}

// Protocol is an API-family adapter such as OpenAI Chat Completions or
// Anthropic Messages. It owns semantic request lowering and stream parsing;
// routes own deployment concerns like URL, auth, and retries.
type Protocol interface {
	API() models.API
	Prepare(ctx context.Context, ref ModelRef, req models.Request) (*PreparedBody, error)
	ParseStream(ctx context.Context, ref ModelRef, resp *http.Response, out *models.EventStream[models.StreamPart, *models.Message])
	CountTokens(ctx context.Context, ref ModelRef, msgs []models.Message) (int, error)
}

// Endpoint renders the request URL for a model reference.
type Endpoint interface {
	URL(ref ModelRef) (string, error)
}

// Auth applies authentication headers to a prepared request.
type Auth interface {
	Apply(req *PreparedRequest) error
}

// Transport executes prepared requests and returns an open streaming response.
type Transport interface {
	Do(ctx context.Context, req *PreparedRequest) (*http.Response, error)
}

// Route is a runnable composition of protocol, endpoint, auth, and transport.
type Route struct {
	ID        string
	Provider  string
	Protocol  Protocol
	Endpoint  Endpoint
	Auth      Auth
	Transport Transport
}

// Prepare lowers a provider-neutral models.Request into a transport-ready HTTP
// request without sending it.
func (r Route) Prepare(ctx context.Context, ref ModelRef, req models.Request) (*PreparedRequest, error) {
	if r.Protocol == nil {
		return nil, fmt.Errorf("route %q has no protocol", r.ID)
	}
	if r.Endpoint == nil {
		return nil, fmt.Errorf("route %q has no endpoint", r.ID)
	}
	prepared, err := r.Protocol.Prepare(ctx, ref, req)
	if err != nil {
		return nil, err
	}
	url, err := r.Endpoint.URL(ref)
	if err != nil {
		return nil, err
	}
	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	copyHeader(h, ref.Headers)
	if prepared != nil {
		copyHeader(h, prepared.Headers)
	}
	out := &PreparedRequest{
		Method:  http.MethodPost,
		URL:     url,
		Headers: h,
		Meta:    map[string]any{},
	}
	if prepared != nil {
		out.Body = prepared.Body
		out.Meta = prepared.Meta
	}
	if r.Auth != nil {
		if err := r.Auth.Apply(out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Stream prepares and executes the route, returning the normalized model event
// stream. Setup failures are returned directly; stream-time failures are the
// protocol parser's responsibility.
func (r Route) Stream(ctx context.Context, ref ModelRef, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	if r.Transport == nil {
		return nil, fmt.Errorf("route %q has no transport", r.ID)
	}
	prepared, err := r.Prepare(ctx, ref, req)
	if err != nil {
		return nil, err
	}
	resp, err := r.Transport.Do(ctx, prepared)
	if err != nil {
		return nil, err
	}
	out := models.NewEventStream[models.StreamPart, *models.Message](64)
	go r.Protocol.ParseStream(ctx, ref, resp, out)
	return out, nil
}

// Model adapts a Route plus ModelRef to the existing models.Model interface.
type Model struct {
	Route Route
	Ref   ModelRef
}

func (m *Model) Info() models.ModelInfo { return m.Ref.Info }

func (m *Model) Stream(ctx context.Context, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	return m.Route.Stream(ctx, m.Ref, req)
}

func (m *Model) CountTokens(ctx context.Context, msgs []models.Message) (int, error) {
	return m.Route.Protocol.CountTokens(ctx, m.Ref, msgs)
}

func copyHeader(dst, src http.Header) {
	for k, values := range src {
		for _, value := range values {
			dst.Add(k, value)
		}
	}
}

var _ models.Model = (*Model)(nil)
