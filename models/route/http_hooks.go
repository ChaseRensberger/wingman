package route

import (
	"context"
	"net/http"

	"github.com/chaserensberger/wingman/models"
)

// HTTPHooks intercept physical model HTTP exchanges before status handling.
type HTTPHooks struct {
	Request  func(context.Context, models.ModelRef, *http.Request) (*http.Request, error)
	Response func(context.Context, models.ModelRef, *http.Request, *http.Response) (*http.Response, error)
}

type httpHooksKey struct{}

// WithHTTPHooks attaches request-owned interceptors without changing shared transports.
func WithHTTPHooks(ctx context.Context, hooks HTTPHooks) context.Context {
	return context.WithValue(ctx, httpHooksKey{}, hooks)
}
