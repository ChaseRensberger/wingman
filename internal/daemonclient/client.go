// Package daemonclient discovers and verifies a local Wingman daemon.
package daemonclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/internal/daemonstate"
)

// Status describes the verified state of a discovered daemon.
type Status string

const (
	StatusMissing      Status = "missing"
	StatusStarting     Status = "starting"
	StatusReady        Status = "ready"
	StatusStale        Status = "stale"
	StatusIncompatible Status = "incompatible"
)

const startingWindow = 30 * time.Second

// Result contains the discovered registration and authenticated readiness.
type Result struct {
	Status       Status
	Registration daemonstate.Registration
	Readiness    api.ReadinessResponse
	Err          error
}

// Inspect reads local state and authenticates the registered daemon.
func Inspect(ctx context.Context, state *daemonstate.State, expectedVersion string) Result {
	registration, err := state.ReadRegistration()
	if err != nil {
		status := StatusStale
		if errors.Is(err, os.ErrNotExist) {
			status = StatusMissing
		}
		return Result{Status: status, Err: err}
	}
	credential, err := state.ReadCredential()
	if err != nil {
		return Result{Status: StatusStale, Registration: registration, Err: err}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(registration.URL, "/")+"/ready", nil)
	if err != nil {
		return Result{Status: StatusStale, Registration: registration, Err: err}
	}
	request.Header.Set("Authorization", "Bearer "+credential)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		status := StatusStale
		if created, parseErr := time.Parse(time.RFC3339Nano, registration.CreatedAt); parseErr == nil {
			age := time.Since(created)
			if age >= 0 && age < startingWindow {
				status = StatusStarting
			}
		}
		return Result{Status: status, Registration: registration, Err: err}
	}
	defer response.Body.Close()

	var readiness api.ReadinessResponse
	if err := json.NewDecoder(response.Body).Decode(&readiness); err != nil {
		return Result{Status: StatusStale, Registration: registration, Err: fmt.Errorf("decode readiness: %w", err)}
	}
	result := Result{Registration: registration, Readiness: readiness}
	if readiness.InstanceID != registration.InstanceID {
		result.Status = StatusStale
		result.Err = errors.New("readiness instance does not match registration")
		return result
	}
	if readiness.Version != registration.Version || expectedVersion != "" && readiness.Version != expectedVersion {
		result.Status = StatusIncompatible
		result.Err = fmt.Errorf("daemon version %q does not match client version %q", readiness.Version, expectedVersion)
		return result
	}
	if response.StatusCode == http.StatusOK && readiness.Ready {
		result.Status = StatusReady
		return result
	}
	if response.StatusCode == http.StatusServiceUnavailable && !readiness.Ready {
		result.Status = StatusStarting
		return result
	}
	result.Status = StatusStale
	result.Err = fmt.Errorf("readiness returned HTTP %d", response.StatusCode)
	return result
}

// WaitReady polls discovery until a compatible daemon is ready or ctx ends.
func WaitReady(ctx context.Context, state *daemonstate.State, expectedVersion string, interval time.Duration) (daemonstate.Registration, error) {
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		result := Inspect(ctx, state, expectedVersion)
		switch result.Status {
		case StatusReady:
			return result.Registration, nil
		case StatusIncompatible:
			return daemonstate.Registration{}, result.Err
		}
		select {
		case <-ctx.Done():
			return daemonstate.Registration{}, fmt.Errorf("wait for daemon readiness: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
