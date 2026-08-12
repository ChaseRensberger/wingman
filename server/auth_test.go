package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestAuthenticationIsDisabledWithoutPassword(t *testing.T) {
	s := New(Config{})
	t.Cleanup(func() { _ = s.Close(context.Background()) })

	response := httptest.NewRecorder()
	s.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func jsonRequest(method, path string, value any) *http.Request {
	body, _ := json.Marshal(value)
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func authenticatedRequest(method, path, password string) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	request.SetBasicAuth("wingman", password)
	return request
}

func TestAuthenticationAcceptsConfiguredBasicAuth(t *testing.T) {
	s := New(Config{Username: "service-user", Password: "secret"})
	t.Cleanup(func() { _ = s.Close(context.Background()) })

	unauthorized := httptest.NewRecorder()
	s.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}
	bearer := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	request.Header.Set("Authorization", "Bearer secret")
	s.ServeHTTP(bearer, request)
	if bearer.Code != http.StatusUnauthorized {
		t.Fatalf("bearer status = %d", bearer.Code)
	}

	authorized := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/ready", nil)
	request.SetBasicAuth("service-user", "secret")
	s.ServeHTTP(authorized, request)
	if authorized.Code != http.StatusServiceUnavailable {
		t.Fatalf("authorized status = %d", authorized.Code)
	}
}

func TestConsoleRequiresBasicAuth(t *testing.T) {
	s := New(Config{Password: "secret"})
	t.Cleanup(func() { _ = s.Close(context.Background()) })

	unauthorized := httptest.NewRecorder()
	s.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/console/", nil))
	if unauthorized.Code != http.StatusUnauthorized || unauthorized.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("unauthorized console = %d, challenge = %q", unauthorized.Code, unauthorized.Header().Get("WWW-Authenticate"))
	}

	authorized := httptest.NewRecorder()
	s.ServeHTTP(authorized, authenticatedRequest(http.MethodGet, "/console/", "secret"))
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized console = %d", authorized.Code)
	}
}

func TestClientRegistrationDoesNotIssueToken(t *testing.T) {
	s := New(Config{Password: "secret", Store: memory.NewStore()})
	t.Cleanup(func() { _ = s.Close(context.Background()) })

	request := jsonRequest(http.MethodPost, "/clients", api.CreateClientRequest{ID: "cli_test", Name: "Test"})
	request.SetBasicAuth("wingman", "secret")
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	var created api.CreateClientResponse
	if response.Code != http.StatusCreated || json.NewDecoder(response.Body).Decode(&created) != nil || created.Client.ID != "cli_test" {
		t.Fatalf("create client status = %d: %s", response.Code, response.Body.String())
	}
}
