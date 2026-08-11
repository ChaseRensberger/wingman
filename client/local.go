package client

import (
	"context"
	"fmt"

	"github.com/chaserensberger/wingman/internal/daemonclient"
	"github.com/chaserensberger/wingman/internal/daemonstate"
)

// NewLocal discovers and verifies the managed daemon for the current user.
func NewLocal(ctx context.Context, options ...Option) (*SDK, error) {
	dir, err := daemonstate.DefaultDir()
	if err != nil {
		return nil, fmt.Errorf("resolve managed daemon state directory: %w", err)
	}
	return NewLocalFromState(ctx, dir, options...)
}

// NewLocalFromState discovers and verifies a managed daemon at stateDir.
func NewLocalFromState(ctx context.Context, stateDir string, options ...Option) (*SDK, error) {
	state := daemonstate.New(stateDir)
	result := daemonclient.Inspect(ctx, state, "")
	if result.Status != daemonclient.StatusReady {
		if result.Err != nil {
			return nil, fmt.Errorf("managed daemon is %s: %w", result.Status, result.Err)
		}
		return nil, fmt.Errorf("managed daemon is %s", result.Status)
	}
	password, err := state.ReadPassword()
	if err != nil {
		return nil, fmt.Errorf("read managed daemon password: %w", err)
	}
	options = append(append([]Option{}, options...), WithPassword(password))
	return New(result.Registration.URL, options...)
}
