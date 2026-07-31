package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/api"
	"github.com/go-chi/chi/v5/middleware"
)

func TestErrorResponsesUseCanonicalEnvelope(t *testing.T) {
	s := New(Config{})
	request := httptest.NewRequest(http.MethodGet, "/missing", nil)
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	var body api.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != api.ErrorCodeNotFound || body.Error.Message != "route not found" {
		t.Fatalf("error = %#v", body.Error)
	}
	if body.Error.RequestID == "" || response.Header().Get("X-Request-ID") != body.Error.RequestID {
		t.Fatalf("request IDs: header=%q body=%q", response.Header().Get("X-Request-ID"), body.Error.RequestID)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q", got)
	}
}

func TestInternalErrorResponsesHideFailureDetails(t *testing.T) {
	var logs bytes.Buffer
	s := New(Config{Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	s.router.Get("/failure", func(w http.ResponseWriter, _ *http.Request) {
		s.writeError(w, http.StatusInternalServerError, "database password leaked")
	})
	request := httptest.NewRequest(http.MethodGet, "/failure", nil)
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)

	var body api.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != api.ErrorCodeInternal || body.Error.Message != "internal server error" {
		t.Fatalf("error = %#v", body.Error)
	}
	if !strings.Contains(logs.String(), "database password leaked") || !strings.Contains(logs.String(), body.Error.RequestID) {
		t.Fatalf("configured logs do not contain correlated failure: %s", logs.String())
	}
}

func TestPanicsUseCanonicalErrorEnvelope(t *testing.T) {
	s := New(Config{})
	s.router.Get("/panic", func(http.ResponseWriter, *http.Request) { panic("secret") })
	request := httptest.NewRequest(http.MethodGet, "/panic", nil)
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)

	var body api.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusInternalServerError || body.Error.Code != api.ErrorCodeInternal || body.Error.Message != "internal server error" {
		t.Fatalf("status = %d, error = %#v", response.Code, body.Error)
	}
}

func TestTimeoutUsesCanonicalErrorEnvelope(t *testing.T) {
	s := New(Config{})
	handler := middleware.RequestID(requestIDHeader(s.requestLogger(s.timeoutWithBypass(time.Millisecond, nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})))))
	request := httptest.NewRequest(http.MethodGet, "/slow", nil).WithContext(context.Background())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	var body api.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusGatewayTimeout || body.Error.Code != api.ErrorCodeTimeout || body.Error.Message != "request timed out" {
		t.Fatalf("status = %d, error = %#v", response.Code, body.Error)
	}
	if body.Error.RequestID == "" || response.Header().Get("X-Request-ID") != body.Error.RequestID {
		t.Fatalf("request IDs: header=%q body=%q", response.Header().Get("X-Request-ID"), body.Error.RequestID)
	}
}

func TestPanicDoesNotAppendErrorAfterCommittedResponse(t *testing.T) {
	s := New(Config{})
	s.router.Get("/partial-panic", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("partial"))
		panic("secret")
	})
	response := httptest.NewRecorder()
	s.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/partial-panic", nil))
	if response.Code != http.StatusAccepted || response.Body.String() != "partial" {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
}

func TestMethodNotAllowedIncludesAllowHeader(t *testing.T) {
	s := New(Config{})
	response := httptest.NewRecorder()
	s.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/health", nil))
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET" {
		t.Fatalf("status = %d, allow = %q", response.Code, response.Header().Get("Allow"))
	}
}

func TestRunStreamErrorsUseCanonicalEnvelope(t *testing.T) {
	event, err := canonicalRunStreamEvent(session.StreamEvent{
		Type: "error", Version: session.EnvelopeVersion, Data: map[string]string{"error": "provider failed"},
	}, "req_test")
	if err != nil {
		t.Fatal(err)
	}
	failure, ok := event.Data.(api.RunErrorEventData)
	if !ok {
		t.Fatalf("error data type = %T", event.Data)
	}
	if failure.Code != api.ErrorCodeRunFailed || failure.Message != "provider failed" || failure.RequestID != "req_test" {
		t.Fatalf("error = %#v", failure)
	}
}
