package httpmodel

import (
	"net/http"
	"net/url"
	"strings"
)

// Endpoint identifies the HTTP deployment behind a provider route.
type Endpoint struct {
	BaseURL string
	Query   map[string]string
}

// Route composes the orthogonal pieces of a model deployment: protocol,
// endpoint, auth, and static headers. Protocol code still owns request body
// lowering and stream parsing.
type Route struct {
	ID       string
	Protocol Protocol
	Endpoint Endpoint
	Auth     Auth
	Headers  map[string]string
}

func (r Route) URL() string {
	base := strings.TrimRight(r.Endpoint.BaseURL, "/")
	path := ""
	switch r.Protocol {
	case OpenAIResponses:
		path = "/responses"
	case OpenAIChat:
		path = "/chat/completions"
	case AnthropicMessages:
		path = "/messages"
	}
	raw := base + path
	if len(r.Endpoint.Query) == 0 {
		return raw
	}
	values := url.Values{}
	for k, v := range r.Endpoint.Query {
		values.Set(k, v)
	}
	return raw + "?" + values.Encode()
}

func (r Route) Apply(req *http.Request) error {
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	if r.Auth == nil {
		return nil
	}
	return r.Auth.Apply(req)
}

func routeHeaders(protocol Protocol) map[string]string {
	if protocol != AnthropicMessages {
		return nil
	}
	return map[string]string{
		"anthropic-version": "2023-06-01",
		"anthropic-beta":    "interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14",
	}
}
