package plugin

import (
	"context"
	"net/http"
	"net/url"

	"github.com/chaserensberger/wingman/api"
)

// Sessions calls Wingman's session API in the current client scope.
type Sessions struct{ host *Host }

// Sessions returns the typed session operations available to a plugin.
func (h *Host) Sessions() Sessions { return Sessions{host: h} }

// Create creates a session owned by the invoking client.
func (s Sessions) Create(ctx context.Context, input api.CreateSessionRequest) (*api.Session, error) {
	var output api.Session
	err := s.host.Request(ctx, http.MethodPost, "/sessions", input, &output)
	return &output, err
}

// Get reads a session and its conversation.
func (s Sessions) Get(ctx context.Context, id string) (*api.SessionDetail, error) {
	var output api.SessionDetail
	err := s.host.Request(ctx, http.MethodGet, "/sessions/"+url.PathEscape(id), nil, &output)
	return &output, err
}

// List lists the invoking client's sessions.
func (s Sessions) List(ctx context.Context) ([]api.Session, error) {
	var output []api.Session
	err := s.host.Request(ctx, http.MethodGet, "/sessions", nil, &output)
	return output, err
}

// Prompt admits a turn without waiting for its model execution.
func (s Sessions) Prompt(ctx context.Context, id string, input api.MessageSessionRequest) (*api.MessageSessionResponse, error) {
	var output api.MessageSessionResponse
	err := s.host.Request(ctx, http.MethodPost, "/sessions/"+url.PathEscape(id)+"/message", input, &output)
	return &output, err
}

// Run reads the durable state of an admitted turn.
func (s Sessions) Run(ctx context.Context, sessionID, runID string) (*api.SessionRun, error) {
	var output api.SessionRun
	err := s.host.Request(ctx, http.MethodGet, "/sessions/"+url.PathEscape(sessionID)+"/runs/"+url.PathEscape(runID), nil, &output)
	return &output, err
}

// Abort requests cancellation without replaying unfinished actions.
func (s Sessions) Abort(ctx context.Context, id string) error {
	return s.host.Request(ctx, http.MethodPost, "/sessions/"+url.PathEscape(id)+"/abort", nil, nil)
}
