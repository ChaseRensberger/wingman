// Package daemonclient discovers and verifies a local Wingman daemon.
package daemonclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// Client is an authenticated transport to a verified managed daemon.
type Client struct {
	baseURL    *url.URL
	credential string
	httpClient *http.Client
}

// APIError reports a non-success response from the daemon API.
type APIError struct {
	StatusCode int
	Response   api.Error
}

func (e *APIError) Error() string {
	message := strings.TrimSpace(e.Response.Message)
	if message == "" {
		message = http.StatusText(e.StatusCode)
	}
	if e.Response.Code != "" {
		return fmt.Sprintf("daemon API returned HTTP %d (%s): %s", e.StatusCode, e.Response.Code, message)
	}
	return fmt.Sprintf("daemon API returned HTTP %d: %s", e.StatusCode, message)
}

// New discovers a ready compatible managed daemon and returns its authenticated client.
func New(ctx context.Context, state *daemonstate.State, expectedVersion string) (*Client, error) {
	result := Inspect(ctx, state, expectedVersion)
	if result.Status != StatusReady {
		return nil, unavailableError(result)
	}
	credential, err := state.ReadCredential()
	if err != nil {
		return nil, fmt.Errorf("read managed daemon credential: %w", err)
	}
	baseURL, err := url.Parse(result.Registration.URL)
	if err != nil {
		return nil, fmt.Errorf("parse managed daemon URL: %w", err)
	}
	return &Client{baseURL: baseURL, credential: credential, httpClient: http.DefaultClient}, nil
}

// URL returns the registered daemon URL.
func (c *Client) URL() string {
	return c.baseURL.String()
}

// DoJSON makes an authenticated JSON request. requestBody and responseBody may be nil.
func (c *Client) DoJSON(ctx context.Context, method, path string, requestBody, responseBody any) error {
	endpoint, err := c.baseURL.Parse(path)
	if err != nil {
		return fmt.Errorf("resolve daemon API path: %w", err)
	}
	if endpoint.Host != c.baseURL.Host || endpoint.Scheme != c.baseURL.Scheme {
		return errors.New("daemon API path must resolve to the managed daemon")
	}

	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("encode daemon API request: %w", err)
		}
		body = strings.NewReader(string(encoded))
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return fmt.Errorf("create daemon API request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.credential)
	request.Header.Set("Accept", "application/json")
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("request managed daemon API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var errorResponse api.ErrorResponse
		if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&errorResponse); err == nil {
			return &APIError{StatusCode: response.StatusCode, Response: errorResponse.Error}
		}
		return &APIError{StatusCode: response.StatusCode}
	}
	if responseBody == nil {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(responseBody); err != nil {
		return fmt.Errorf("decode daemon API response: %w", err)
	}
	return nil
}

func unavailableError(result Result) error {
	switch result.Status {
	case StatusMissing:
		return errors.New("no managed Wingman daemon found; run 'wingman up'")
	case StatusStarting:
		return errors.New("managed Wingman daemon is starting; wait a moment and try again")
	case StatusIncompatible:
		return daemonUnavailableError("managed Wingman daemon is incompatible; run 'wingman update' or restart it", result.Err)
	default:
		return daemonUnavailableError("managed Wingman daemon registration is stale or unreachable; run 'wingman restart'", result.Err)
	}
}

func daemonUnavailableError(message string, cause error) error {
	if cause == nil {
		return errors.New(message)
	}
	return fmt.Errorf("%s: %w", message, cause)
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
