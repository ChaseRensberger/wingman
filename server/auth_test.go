package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chaserensberger/wingman/api"
)

func TestAuthenticationAndReadiness(t *testing.T) {
	s := New(Config{Credential: "secret", InstanceID: "instance_test", Version: "1.2.3"})
	t.Cleanup(func() { _ = s.Close(context.Background()) })

	health := httptest.NewRecorder()
	s.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d", health.Code)
	}

	unauthorized := httptest.NewRecorder()
	s.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if unauthorized.Code != http.StatusUnauthorized || unauthorized.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("unauthorized status = %d, challenge = %q", unauthorized.Code, unauthorized.Header().Get("WWW-Authenticate"))
	}

	before := authenticatedRequest(http.MethodGet, "/ready", "secret")
	response := httptest.NewRecorder()
	s.ServeHTTP(response, before)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("starting readiness status = %d", response.Code)
	}

	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	s.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/ready", "secret"))
	if response.Code != http.StatusOK {
		t.Fatalf("ready status = %d, body = %s", response.Code, response.Body.String())
	}
	var ready api.ReadinessResponse
	if err := json.NewDecoder(response.Body).Decode(&ready); err != nil {
		t.Fatal(err)
	}
	if !ready.Ready || ready.InstanceID != "instance_test" || ready.Version != "1.2.3" {
		t.Fatalf("readiness = %#v", ready)
	}
}

func TestLoopbackConsoleSessionCookieAuthenticates(t *testing.T) {
	s := New(Config{Credential: "secret", InstanceID: "instance_test", Version: "dev", ConsoleCookie: true})
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close(context.Background()) })

	consoleRequest := httptest.NewRequest(http.MethodGet, "/console", nil)
	consoleRequest.Host = "127.0.0.1:2323"
	console := httptest.NewRecorder()
	s.ServeHTTP(console, consoleRequest)
	result := console.Result()
	var session *http.Cookie
	for _, cookie := range result.Cookies() {
		if cookie.Name == consoleSessionCookie {
			session = cookie
		}
	}
	if session == nil || session.Value != "secret" || !session.HttpOnly || session.SameSite != http.SameSiteStrictMode || session.Path != "/" {
		t.Fatalf("console session cookie = %#v", session)
	}

	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	request.AddCookie(session)
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("cookie-authenticated readiness status = %d", response.Code)
	}
}

func TestConsoleDoesNotBootstrapCookieForUntrustedHost(t *testing.T) {
	s := New(Config{Credential: "secret", ConsoleCookie: true})
	request := httptest.NewRequest(http.MethodGet, "/console", nil)
	request.Host = "attacker.example:2323"
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == consoleSessionCookie {
			t.Fatal("console session cookie was set for an untrusted host")
		}
	}
}

func TestRequestHostIsLoopback(t *testing.T) {
	for _, host := range []string{"localhost", "localhost:2323", "127.0.0.1:2323", "[::1]:2323"} {
		if !requestHostIsLoopback(host) {
			t.Errorf("requestHostIsLoopback(%q) = false", host)
		}
	}
	for _, host := range []string{"", "attacker.example", "attacker.example:2323", "192.0.2.1:2323"} {
		if requestHostIsLoopback(host) {
			t.Errorf("requestHostIsLoopback(%q) = true", host)
		}
	}
}

func TestConsoleDoesNotBootstrapCookieWhenDisabled(t *testing.T) {
	s := New(Config{Credential: "secret"})
	response := httptest.NewRecorder()
	s.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/console", nil))
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == consoleSessionCookie {
			t.Fatal("console session cookie was set")
		}
	}
}

func authenticatedRequest(method, path, credential string) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	request.Header.Set("Authorization", "Bearer "+credential)
	return request
}
