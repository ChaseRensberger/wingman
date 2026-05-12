package route

import (
	"fmt"
	"net/url"
	"strings"
)

type pathEndpoint string

type endpointFunc func(ModelRef) (string, error)

// Path returns an endpoint that appends path to ModelRef.BaseURL.
func Path(path string) Endpoint { return pathEndpoint(path) }

// EndpointFunc adapts a function into an Endpoint.
func EndpointFunc(fn func(ModelRef) (string, error)) Endpoint { return endpointFunc(fn) }

func (p pathEndpoint) URL(ref ModelRef) (string, error) {
	if strings.TrimSpace(ref.BaseURL) == "" {
		return "", fmt.Errorf("route endpoint: missing base URL")
	}
	base := strings.TrimRight(ref.BaseURL, "/")
	path := "/" + strings.TrimLeft(string(p), "/")
	return base + path, nil
}

func (fn endpointFunc) URL(ref ModelRef) (string, error) { return fn(ref) }

// JoinPath appends path to base using URL path escaping for path segments the
// caller has already escaped or intentionally left raw.
func JoinPath(base, path string) (string, error) {
	if strings.TrimSpace(base) == "" {
		return "", fmt.Errorf("route endpoint: missing base URL")
	}
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil {
		return "", fmt.Errorf("route endpoint: parse base URL: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(path, "/")
	return u.String(), nil
}
