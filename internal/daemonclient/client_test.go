package daemonclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
