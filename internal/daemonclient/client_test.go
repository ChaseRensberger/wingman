package daemonclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/internal/daemonstate"
)

func TestInspect(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	tests := []struct {
		name            string
		statusCode      int
		readiness       api.ReadinessResponse
		expectedVersion string
		want            Status
	}{
		{name: "ready", statusCode: http.StatusOK, readiness: api.ReadinessResponse{Ready: true, InstanceID: "one", Version: "1.0.0"}, expectedVersion: "1.0.0", want: StatusReady},
		{name: "starting", statusCode: http.StatusServiceUnavailable, readiness: api.ReadinessResponse{Ready: false, InstanceID: "one", Version: "1.0.0"}, expectedVersion: "1.0.0", want: StatusStarting},
		{name: "instance mismatch", statusCode: http.StatusOK, readiness: api.ReadinessResponse{Ready: true, InstanceID: "other", Version: "1.0.0"}, expectedVersion: "1.0.0", want: StatusStale},
		{name: "version mismatch", statusCode: http.StatusOK, readiness: api.ReadinessResponse{Ready: true, InstanceID: "one", Version: "2.0.0"}, expectedVersion: "1.0.0", want: StatusIncompatible},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := daemonstate.New(t.TempDir())
			config, err := daemonstate.EnsureServiceConfig()
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				username, supplied, ok := r.BasicAuth()
				if !ok || username != config.Username || supplied != config.Password {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.WriteHeader(test.statusCode)
				_ = json.NewEncoder(w).Encode(test.readiness)
			}))
			defer server.Close()
			registration := daemonstate.Registration{InstanceID: "one", Version: test.readiness.Version, URL: server.URL, PID: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
			if err := state.WriteRegistration(registration); err != nil {
				t.Fatal(err)
			}
			if result := Inspect(context.Background(), state, test.expectedVersion); result.Status != test.want {
				t.Fatalf("Inspect() = %s, %v; want %s", result.Status, result.Err, test.want)
			}
		})
	}
}

func TestInspectMissingAndStale(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	state := daemonstate.New(t.TempDir())
	if result := Inspect(context.Background(), state, "dev"); result.Status != StatusMissing {
		t.Fatalf("missing Inspect() = %s", result.Status)
	}
	if _, err := daemonstate.EnsureServiceConfig(); err != nil {
		t.Fatal(err)
	}
	registration := daemonstate.Registration{InstanceID: "one", Version: "dev", URL: "http://127.0.0.1:1", PID: 1, CreatedAt: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)}
	if err := state.WriteRegistration(registration); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if result := Inspect(ctx, state, "dev"); result.Status != StatusStale {
		t.Fatalf("stale Inspect() = %s, %v", result.Status, result.Err)
	}
}

func TestInspectFutureRegistrationIsStale(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	state := daemonstate.New(t.TempDir())
	if _, err := daemonstate.EnsureServiceConfig(); err != nil {
		t.Fatal(err)
	}
	registration := daemonstate.Registration{InstanceID: "one", Version: "dev", URL: "http://127.0.0.1:1", PID: 1, CreatedAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)}
	if err := state.WriteRegistration(registration); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if result := Inspect(ctx, state, "dev"); result.Status != StatusStale {
		t.Fatalf("future Inspect() = %s, %v", result.Status, result.Err)
	}
}

func TestClientDoJSONAuthenticatesAndDecodesResponse(t *testing.T) {
	const password = "root-password"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, supplied, ok := r.BasicAuth()
		if !ok || username != "wingman" || supplied != password {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		if r.URL.Path != "/clients" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		var request api.CreateClientRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.ID != "cli_one" || request.Name != "One" {
			t.Fatalf("client = %#v", request)
		}
		_ = json.NewEncoder(w).Encode(api.CreateClientResponse{Client: api.Client{ID: "cli_one", Name: "One"}})
	}))
	defer server.Close()
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{baseURL: baseURL, username: "wingman", password: password, httpClient: server.Client()}
	var created api.CreateClientResponse
	if err := client.DoJSON(context.Background(), http.MethodPost, "/clients", api.CreateClientRequest{ID: "cli_one", Name: "One"}, &created); err != nil {
		t.Fatal(err)
	}
	if created.Client.ID != "cli_one" {
		t.Fatalf("client = %#v", created.Client)
	}
}

func TestClientDoJSONReturnsAPIErrorWithoutPassword(t *testing.T) {
	const password = "root-password"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: api.Error{Code: api.ErrorCodeForbidden, Message: "access denied"}})
	}))
	defer server.Close()
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{baseURL: baseURL, username: "wingman", password: password, httpClient: server.Client()}
	err = client.DoJSON(context.Background(), http.MethodGet, "/clients", nil, nil)
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("error = %v, want APIError", err)
	}
	if apiError.StatusCode != http.StatusForbidden || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), password) {
		t.Fatalf("error leaked password: %v", err)
	}
}

func TestInspectRejectsNonLoopbackRegistrationBeforeReadingPassword(t *testing.T) {
	state := daemonstate.New(t.TempDir())
	registration := daemonstate.Registration{InstanceID: "one", Version: "dev", URL: "https://example.com", PID: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := state.WriteRegistration(registration); err != nil {
		t.Fatal(err)
	}

	result := Inspect(context.Background(), state, "dev")
	if result.Status != StatusStale {
		t.Fatalf("status = %s, want %s", result.Status, StatusStale)
	}
	if result.Err == nil || !strings.Contains(result.Err.Error(), "loopback") {
		t.Fatalf("error = %v", result.Err)
	}
}

func TestInspectDoesNotFollowRedirects(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected.Add(1)
	}))
	defer target.Close()

	state := daemonstate.New(t.TempDir())
	config, err := daemonstate.EnsureServiceConfig()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		username, supplied, ok := request.BasicAuth()
		if !ok || username != config.Username || supplied != config.Password {
			t.Fatalf("basic auth = %q, %q, %t", username, supplied, ok)
		}
		http.Redirect(response, request, target.URL, http.StatusFound)
	}))
	defer server.Close()
	registration := daemonstate.Registration{InstanceID: "one", Version: "dev", URL: server.URL, PID: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := state.WriteRegistration(registration); err != nil {
		t.Fatal(err)
	}

	result := Inspect(context.Background(), state, "dev")
	if result.Status != StatusStale {
		t.Fatalf("status = %s, want %s", result.Status, StatusStale)
	}
	if redirected.Load() != 0 {
		t.Fatalf("redirect target requests = %d", redirected.Load())
	}
}
