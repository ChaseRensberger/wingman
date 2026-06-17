package httpmodel

import (
	"fmt"
	"net/http"
)

// Auth mutates an outbound provider request with authentication headers.
// It is intentionally composable so provider routes can express direct API
// keys, gateways, custom headers, and no-auth endpoints without protocol
// special cases.
type Auth interface {
	Apply(*http.Request) error
}

type AuthFunc func(*http.Request) error

func (f AuthFunc) Apply(req *http.Request) error { return f(req) }

// NoAuth leaves the request unchanged.
var NoAuth Auth = AuthFunc(func(*http.Request) error { return nil })

// HeaderAuth sets one header to a fixed value when value is non-empty.
func HeaderAuth(name, value string) Auth {
	return AuthFunc(func(req *http.Request) error {
		if name == "" {
			return fmt.Errorf("auth header name is required")
		}
		if value == "" {
			return nil
		}
		req.Header.Set(name, value)
		return nil
	})
}

// BearerAuth sets Authorization: Bearer <token> when token is non-empty.
func BearerAuth(token string) Auth {
	if token == "" {
		return NoAuth
	}
	return HeaderAuth("authorization", "Bearer "+token)
}

// ChainAuth applies each auth step in order.
func ChainAuth(auths ...Auth) Auth {
	return AuthFunc(func(req *http.Request) error {
		for _, auth := range auths {
			if auth == nil {
				continue
			}
			if err := auth.Apply(req); err != nil {
				return err
			}
		}
		return nil
	})
}

func defaultAuth(protocol Protocol, apiKey string) Auth {
	if apiKey == "" {
		return NoAuth
	}
	if protocol == AnthropicMessages {
		return HeaderAuth("x-api-key", apiKey)
	}
	return BearerAuth(apiKey)
}
