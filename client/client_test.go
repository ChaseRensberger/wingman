package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/internal/daemonstate"
)

func TestNewAuthenticatesGeneratedRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/" {
			t.Errorf("path = %q", request.URL.Path)
		}
		username, password, ok := request.BasicAuth()
		if !ok || username != "wingman" || password != "secret" {
			t.Errorf("basic auth = %q, %q, %t", username, password, ok)
		}
		if clientID := request.Header.Get("X-Wingman-Client"); clientID != "cli_test" {
			t.Errorf("X-Wingman-Client = %q", clientID)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"name":"wingman","status":"ok","health":"/health","console":"/console"}`))
	}))
	defer server.Close()

	client, err := New(server.URL, WithPassword("secret"), WithClientID("cli_test"))
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.GetServiceWithResponse(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.JSON200 == nil || response.JSON200.Name != "wingman" {
		t.Fatalf("response = %#v", response)
	}
}

func TestGeneratedRequestsReturnAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("X-Request-ID", "req_header")
		response.Header().Set("Retry-After", "3")
		response.WriteHeader(http.StatusUnauthorized)
		_, _ = response.Write([]byte(`{"error":{"code":"unauthorized","message":"bad password"}}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetService(context.Background(), nil)
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("error = %v, want APIError", err)
	}
	if apiError.StatusCode != http.StatusUnauthorized || apiError.Response.Code != api.ErrorCodeUnauthorized {
		t.Fatalf("APIError = %#v", apiError)
	}
	if apiError.RequestID != "req_header" || apiError.RetryAfter != 3*time.Second || apiError.Headers.Get("Retry-After") != "3" {
		t.Fatalf("APIError diagnostics = %#v", apiError)
	}
}

func TestPerCallClientIDOverridesDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if clientID := request.Header.Get("X-Wingman-Client"); clientID != "cli_override" {
			t.Errorf("X-Wingman-Client = %q", clientID)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"name":"wingman","status":"ok","health":"/health","console":"/console"}`))
	}))
	defer server.Close()

	client, err := New(server.URL, WithClientID("cli_default"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetService(context.Background(), nil, func(_ context.Context, request *http.Request) error {
		request.Header.Set("X-Wingman-Client", "cli_override")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/run" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if accept := request.Header.Get("Accept"); accept != "text/event-stream" {
			t.Errorf("Accept = %q", accept)
		}
		response.Header().Set("Content-Type", "text/event-stream")
		_, _ = response.Write([]byte("event: iteration_start\ndata: {\"type\":\"iteration_start\",\"version\":1,\"data\":{\"step\":1}}\n\n"))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.Run(context.Background(), RunRequest{Message: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if !stream.Next() {
		t.Fatalf("Next() = false, err = %v", stream.Err())
	}
	event := stream.Event()
	if event.Type != api.RunStreamEventIterationStart {
		t.Fatalf("type = %q", event.Type)
	}
	data, ok := event.Data.(*api.RunIterationStartEventData)
	if !ok || data.Step != 1 {
		t.Fatalf("data = %#v", event.Data)
	}
}

func TestSessionEventStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/sessions/session-1/events" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if after := request.URL.Query().Get("after"); after != "7" {
			t.Errorf("after = %q", after)
		}
		response.Header().Set("Content-Type", "text/event-stream")
		_, _ = response.Write([]byte("id: 7\r\nevent: session.events.synchronized\r\ndata: {\"id\":\"7\",\"schema_version\":1,\r\ndata: \"type\":\"session.events.synchronized\",\"data\":{\"cursor\":7,\"watermark\":7}}\r\n\r\n"))
	}))
	defer server.Close()

	after := int64(7)
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.StreamSessionEvents(context.Background(), "session-1", &SessionEventsOptions{After: &after})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if !stream.Next() {
		t.Fatalf("Next() = false, err = %v", stream.Err())
	}
	event := stream.Event()
	if event.Type != api.SessionEventEventsSynchronized {
		t.Fatalf("type = %q", event.Type)
	}
	data, ok := event.Data.(*api.EventsSynchronizedEventData)
	if !ok || data.Cursor != 7 {
		t.Fatalf("data = %#v", event.Data)
	}
	if frame := stream.Frame(); frame.ID != "7" || frame.Event != "session.events.synchronized" {
		t.Fatalf("frame = %#v", frame)
	}
}

func TestListSessionEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/sessions/session-1/events/history" {
			t.Errorf("path = %q", request.URL.Path)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"data":[{"id":"7","schema_version":1,"type":"session.events.synchronized","data":{"cursor":7,"watermark":7}}],"has_more":false}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.ListSessionEvents(context.Background(), "session-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != "7" || page.HasMore {
		t.Fatalf("page = %#v", page)
	}
}

func TestNewRejectsInvalidBaseURL(t *testing.T) {
	for _, baseURL := range []string{"", "localhost:2323", "ftp://example.com", "https://example.com/path"} {
		t.Run(baseURL, func(t *testing.T) {
			if _, err := New(baseURL); err == nil {
				t.Fatal("New() error = nil")
			}
		})
	}
}

func TestNewLocalFromState(t *testing.T) {
	state := daemonstate.New(t.TempDir())
	password, err := state.Password()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		username, supplied, ok := request.BasicAuth()
		if !ok || username != "wingman" || supplied != password {
			t.Fatalf("basic auth = %q, %q, %t", username, supplied, ok)
		}
		_ = json.NewEncoder(response).Encode(api.ReadinessResponse{Ready: true, InstanceID: "ins_1", Version: "0.1.0"})
	}))
	defer server.Close()
	if err := state.WriteRegistration(daemonstate.Registration{InstanceID: "ins_1", Version: "0.1.0", URL: server.URL, PID: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
		t.Fatal(err)
	}

	client, err := NewLocalFromState(context.Background(), state.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if client == nil {
		t.Fatal("client = nil")
	}
}

func TestMessageAdmissionHelpers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/sessions/ses_1/message" {
			t.Errorf("path = %q", request.URL.Path)
		}
		var admission MessageSessionRequest
		if err := json.NewDecoder(request.Body).Decode(&admission); err != nil {
			t.Fatal(err)
		}
		if admission.RequestId == nil || *admission.RequestId == "" {
			t.Error("request ID is empty")
		}
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusAccepted)
		_, _ = response.Write([]byte(`{"run_id":"run_1","status":"queued","session_version":2}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	admission := NewMessageAdmission(MessageSessionRequest{AgentId: "agt_1", Message: "hello"})
	response, err := client.AdmitMessage(context.Background(), "ses_1", admission)
	if err != nil {
		t.Fatal(err)
	}
	if response.RunId != "run_1" {
		t.Fatalf("response = %#v", response)
	}
}

func TestRegisterClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/clients" {
			t.Errorf("path = %q", request.URL.Path)
		}
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusCreated)
		_, _ = response.Write([]byte(`{"client":{"id":"cli_1","name":"Wingcode","created_at":"now"}}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := client.RegisterClient(context.Background(), "cli_1", "Wingcode")
	if err != nil {
		t.Fatal(err)
	}
	if registered.Id != "cli_1" {
		t.Fatalf("client = %#v", registered)
	}
}

func TestSSEDecoderRejectsOversizedEvent(t *testing.T) {
	decoder := newSSEDecoder(io.NopCloser(strings.NewReader("data: 123456789\n\n")), 8)
	if _, ok := decoder.Next(); ok {
		t.Fatal("Next() = true")
	}
	if decoder.Err() == nil {
		t.Fatal("Err() = nil")
	}
}
