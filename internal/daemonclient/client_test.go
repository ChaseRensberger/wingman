package daemonclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/internal/daemonstate"
)

func TestInspect(t *testing.T) {
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
			credential, err := state.Credential()
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+credential {
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
	state := daemonstate.New(t.TempDir())
	if result := Inspect(context.Background(), state, "dev"); result.Status != StatusMissing {
		t.Fatalf("missing Inspect() = %s", result.Status)
	}
	if _, err := state.Credential(); err != nil {
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
	state := daemonstate.New(t.TempDir())
	if _, err := state.Credential(); err != nil {
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
	const credential = "root-credential"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+credential {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		if r.URL.Path != "/auth/pairings" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		var request api.CreatePairingRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.ClientID != "client_one" {
			t.Fatalf("client ID = %q", request.ClientID)
		}
		_ = json.NewEncoder(w).Encode(api.PairingResponse{PairingPath: "/console#pairing=one"})
	}))
	defer server.Close()
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{baseURL: baseURL, credential: credential, httpClient: server.Client()}
	var pairing api.PairingResponse
	if err := client.DoJSON(context.Background(), http.MethodPost, "/auth/pairings", api.CreatePairingRequest{ClientID: "client_one"}, &pairing); err != nil {
		t.Fatal(err)
	}
	if pairing.PairingPath != "/console#pairing=one" {
		t.Fatalf("pairing path = %q", pairing.PairingPath)
	}
}

func TestClientDoJSONReturnsAPIErrorWithoutCredential(t *testing.T) {
	const credential = "root-credential"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: api.Error{Code: api.ErrorCodeForbidden, Message: "access denied"}})
	}))
	defer server.Close()
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{baseURL: baseURL, credential: credential, httpClient: server.Client()}
	err = client.DoJSON(context.Background(), http.MethodGet, "/auth/sessions", nil, nil)
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("error = %v, want APIError", err)
	}
	if apiError.StatusCode != http.StatusForbidden || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), credential) {
		t.Fatalf("error leaked credential: %v", err)
	}
}
