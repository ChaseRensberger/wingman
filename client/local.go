package client

import (
	"context"
	"errors"
	"fmt"

	"github.com/chaserensberger/wingman/internal/daemonclient"
	"github.com/chaserensberger/wingman/internal/daemonstate"
)

// NewLocal discovers and verifies a local daemon for the current user.
func NewLocal(ctx context.Context, options ...Option) (*SDK, error) {
	dir, err := daemonstate.DefaultDir()
	if err != nil {
		return nil, fmt.Errorf("resolve local daemon state directory: %w", err)
	}
	dirs := []string{dir}
	managedDir, err := daemonstate.ManagedDir()
	if err != nil {
		return nil, fmt.Errorf("resolve managed daemon state directory: %w", err)
	}
	if managedDir != dir {
		dirs = append(dirs, managedDir)
	}
	return newLocalFromDirs(ctx, dirs, options...)
}

func newLocalFromDirs(ctx context.Context, dirs []string, options ...Option) (*SDK, error) {
	var errs []error
	for _, dir := range dirs {
		sdk, err := NewLocalFromState(ctx, dir, options...)
		if err == nil {
			return sdk, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", dir, err))
	}
	return nil, fmt.Errorf("discover local Wingman daemon: %w", errors.Join(errs...))
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
