package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
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
	var starting api.ReadinessResponse
	if err := json.NewDecoder(response.Body).Decode(&starting); err != nil {
		t.Fatal(err)
	}
	if starting.Diagnostic == nil || starting.Diagnostic.Subsystem != "startup" || starting.Diagnostic.RecoveryAction != "start the daemon" {
		t.Fatalf("starting readiness = %#v", starting)
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
	if !ready.Ready || ready.InstanceID != "instance_test" || ready.Version != "1.2.3" || ready.Diagnostic != nil {
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
	if session == nil || session.Value == "secret" || !session.HttpOnly || session.SameSite != http.SameSiteStrictMode || session.Path != "/" {
		t.Fatalf("console session cookie = %#v", session)
	}
	if session.MaxAge <= 0 || session.Expires.IsZero() {
		t.Fatalf("console session cookie has no persistent expiry: %#v", session)
	}

	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	request.AddCookie(session)
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("cookie-authenticated readiness status = %d", response.Code)
	}

	assetRequest := httptest.NewRequest(http.MethodGet, "/console/assets/app.js", nil)
	assetRequest.Host = "127.0.0.1:2323"
	assetRequest.AddCookie(session)
	assetResponse := httptest.NewRecorder()
	s.ServeHTTP(assetResponse, assetRequest)
	if len(assetResponse.Result().Cookies()) != 0 {
		t.Fatalf("valid console cookie was replaced: %#v", assetResponse.Result().Cookies())
	}
}

func TestRootCookieIsRejectedAndLocalCookieAuthenticates(t *testing.T) {
	s := New(Config{Credential: "secret", ConsoleCookie: true})
	t.Cleanup(func() { _ = s.Close(context.Background()) })

	rootCookie := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rootCookie.AddCookie(&http.Cookie{Name: consoleSessionCookie, Value: "secret"})
	response := httptest.NewRecorder()
	s.ServeHTTP(response, rootCookie)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("root cookie status = %d", response.Code)
	}

	console := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/console", nil)
	request.Host = "localhost:2323"
	s.ServeHTTP(console, request)
	cookie := console.Result().Cookies()[0]
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/ready", nil)
	request.AddCookie(cookie)
	s.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("auth session cookie status = %d", response.Code)
	}
}

func TestAuthSessionBearerClientBindingAndRevocation(t *testing.T) {
	data := memory.NewStore()
	other, err := data.CreateClient("other")
	if err != nil {
		t.Fatal(err)
	}
	s := New(Config{Credential: "secret", Store: data})
	t.Cleanup(func() { _ = s.Close(context.Background()) })
	token, _, err := s.createAuthSession(other.ID)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	s.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/ready", token))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("bearer session status = %d", response.Code)
	}
	mismatch := authenticatedRequest(http.MethodGet, "/ready", token)
	mismatch.Header.Set("X-Wingman-Client", store.DefaultClientID)
	response = httptest.NewRecorder()
	s.ServeHTTP(response, mismatch)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("mismatched client status = %d", response.Code)
	}
	sessions, err := data.ListAuthSessions(other.ID)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %#v, err = %v", sessions, err)
	}
	if err := data.RevokeAuthSession(sessions[0].ID); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	s.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/ready", token))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session status = %d", response.Code)
	}
}

func TestAuthSessionCannotAccessAnotherClientResources(t *testing.T) {
	data := memory.NewStore()
	owner, err := data.EnsureDefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	pairedClient, err := data.CreateClient("paired")
	if err != nil {
		t.Fatal(err)
	}
	workspace := &store.Workspace{ID: "wsp_owner", Name: "Owner", Path: t.TempDir(), ClientID: owner.ID}
	if err := data.CreateWorkspace(workspace); err != nil {
		t.Fatal(err)
	}
	ownerSession := &store.Session{ID: "ses_owner", ClientID: owner.ID, WorkspaceID: workspace.ID}
	if err := data.CreateSession(ownerSession); err != nil {
		t.Fatal(err)
	}
	pairedSession := &store.Session{ID: "ses_paired", ClientID: pairedClient.ID}
	if err := data.CreateSession(pairedSession); err != nil {
		t.Fatal(err)
	}

	s := New(Config{Credential: "secret", Store: data})
	t.Cleanup(func() { _ = s.Close(context.Background()) })
	token, _, err := s.createAuthSession(pairedClient.ID)
	if err != nil {
		t.Fatal(err)
	}

	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := authenticatedRequest(method, path, token)
		if body != "" {
			req.Body = io.NopCloser(strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		s.ServeHTTP(response, req)
		return response
	}
	for _, test := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "get session", method: http.MethodGet, path: "/sessions/" + ownerSession.ID},
		{name: "delete session", method: http.MethodDelete, path: "/sessions/" + ownerSession.ID + "?expected_version=1"},
		{name: "get workspace", method: http.MethodGet, path: "/workspaces/" + workspace.ID},
		{name: "update workspace", method: http.MethodPut, path: "/workspaces/" + workspace.ID, body: `{"name":"Changed"}`},
		{name: "delete workspace", method: http.MethodDelete, path: "/workspaces/" + workspace.ID},
		{name: "list workspace sessions", method: http.MethodGet, path: "/workspaces/" + workspace.ID + "/sessions"},
		{name: "create session in workspace", method: http.MethodPost, path: "/sessions", body: `{"workspace_id":"wsp_owner"}`},
		{name: "move session to workspace", method: http.MethodPost, path: "/sessions/" + pairedSession.ID + "/move", body: `{"workspace_id":"wsp_owner","expected_version":1}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := request(test.method, test.path, test.body)
			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusForbidden, response.Body.String())
			}
		})
	}

	if _, err := data.GetSession(ownerSession.ID); err != nil {
		t.Fatalf("owner session was deleted: %v", err)
	}
	storedWorkspace, err := data.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedWorkspace.Name != workspace.Name {
		t.Fatalf("workspace name = %q, want %q", storedWorkspace.Name, workspace.Name)
	}
	storedSession, err := data.GetSession(pairedSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedSession.WorkspaceID != "" {
		t.Fatalf("paired session workspace = %q, want empty", storedSession.WorkspaceID)
	}
}

func TestPairingRoutes(t *testing.T) {
	s := New(Config{Credential: "secret"})
	t.Cleanup(func() { _ = s.Close(context.Background()) })

	response := httptest.NewRecorder()
	s.ServeHTTP(response, jsonRequest(http.MethodPost, "/auth/pairings", api.CreatePairingRequest{}))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("public pairing status = %d", response.Code)
	}

	create := jsonRequest(http.MethodPost, "/auth/pairings", api.CreatePairingRequest{})
	create.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	s.ServeHTTP(response, create)
	if response.Code != http.StatusCreated || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("create pairing status = %d, cache = %q", response.Code, response.Header().Get("Cache-Control"))
	}
	var pairing api.PairingResponse
	if err := json.NewDecoder(response.Body).Decode(&pairing); err != nil {
		t.Fatal(err)
	}

	redeem := jsonRequest(http.MethodPost, "/auth/pairings/redeem", api.RedeemPairingRequest{Credential: pairing.Credential})
	redeem.Host = "example.test"
	response = httptest.NewRecorder()
	s.ServeHTTP(response, redeem)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("redeem status = %d", response.Code)
	}
	var cookieResponse api.RedeemPairingResponse
	if err := json.NewDecoder(response.Body).Decode(&cookieResponse); err != nil {
		t.Fatal(err)
	}
	if cookieResponse.Token != "" || len(response.Result().Cookies()) != 1 || !response.Result().Cookies()[0].Secure {
		t.Fatalf("cookie response = %#v, cookies = %#v", cookieResponse, response.Result().Cookies())
	}

	response = httptest.NewRecorder()
	s.ServeHTTP(response, jsonRequest(http.MethodPost, "/auth/pairings/redeem", api.RedeemPairingRequest{Credential: pairing.Credential}))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("reused pairing status = %d", response.Code)
	}
}

func TestCookieAuthenticationRejectsSameSiteCrossOriginMutation(t *testing.T) {
	s := New(Config{Credential: "secret", ConsoleCookie: true})
	t.Cleanup(func() { _ = s.Close(context.Background()) })

	console := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/console", nil)
	request.Host = "localhost:2323"
	s.ServeHTTP(console, request)
	cookie := console.Result().Cookies()[0]

	request = httptest.NewRequest(http.MethodPost, "/plugins/reload", nil)
	request.Host = "localhost:2323"
	request.Header.Set("Origin", "http://attacker.localhost:2323")
	request.Header.Set("Sec-Fetch-Site", "same-site")
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin cookie mutation status = %d", response.Code)
	}
}

func TestCookieRequestOriginPolicy(t *testing.T) {
	for _, test := range []struct {
		name    string
		origin  string
		site    string
		allowed bool
	}{
		{name: "matching origin", origin: "https://wingman.example", site: "same-origin", allowed: true},
		{name: "same origin metadata", site: "same-origin", allowed: true},
		{name: "missing browser metadata", allowed: false},
		{name: "sibling origin", origin: "https://other.example", site: "same-site", allowed: false},
		{name: "cross site", origin: "https://attacker.test", site: "cross-site", allowed: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/sessions", nil)
			request.Host = "wingman.example"
			request.Header.Set("Origin", test.origin)
			request.Header.Set("Sec-Fetch-Site", test.site)
			if got := cookieRequestAllowed(request); got != test.allowed {
				t.Fatalf("cookieRequestAllowed() = %v, want %v", got, test.allowed)
			}
		})
	}
}

func TestPairingRedemptionBodyIsBounded(t *testing.T) {
	s := New(Config{Credential: "secret"})
	t.Cleanup(func() { _ = s.Close(context.Background()) })
	request := httptest.NewRequest(http.MethodPost, "/auth/pairings/redeem", strings.NewReader(`{"credential":"`+strings.Repeat("x", 9<<10)+`"}`))
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized redemption status = %d", response.Code)
	}
}

type failCreateAuthStore struct {
	store.Store
	mu       sync.Mutex
	failOnce bool
}

func (s *failCreateAuthStore) CreateAuthSession(session *store.AuthSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failOnce {
		s.failOnce = false
		return errors.New("database unavailable")
	}
	return s.Store.CreateAuthSession(session)
}

func TestPairingCanRetryAfterSessionPersistenceFailure(t *testing.T) {
	data := &failCreateAuthStore{Store: memory.NewStore(), failOnce: true}
	s := New(Config{Credential: "secret", Store: data})
	t.Cleanup(func() { _ = s.Close(context.Background()) })

	create := jsonRequest(http.MethodPost, "/auth/pairings", api.CreatePairingRequest{})
	create.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	s.ServeHTTP(response, create)
	var pairing api.PairingResponse
	if response.Code != http.StatusCreated || json.NewDecoder(response.Body).Decode(&pairing) != nil {
		t.Fatalf("create pairing status = %d", response.Code)
	}

	for attempt, want := range []int{http.StatusInternalServerError, http.StatusOK} {
		response = httptest.NewRecorder()
		s.ServeHTTP(response, jsonRequest(http.MethodPost, "/auth/pairings/redeem", api.RedeemPairingRequest{Credential: pairing.Credential, Mode: api.AuthSessionModeBearer}))
		if response.Code != want {
			t.Fatalf("redemption attempt %d status = %d, want %d", attempt+1, response.Code, want)
		}
	}
}

func TestPairingBearerAndConcurrentRedeem(t *testing.T) {
	s := New(Config{Credential: "secret"})
	t.Cleanup(func() { _ = s.Close(context.Background()) })
	createPairing := func() api.PairingResponse {
		request := jsonRequest(http.MethodPost, "/auth/pairings", api.CreatePairingRequest{})
		request.Header.Set("Authorization", "Bearer secret")
		response := httptest.NewRecorder()
		s.ServeHTTP(response, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("create status = %d", response.Code)
		}
		var pairing api.PairingResponse
		if err := json.NewDecoder(response.Body).Decode(&pairing); err != nil {
			t.Fatal(err)
		}
		return pairing
	}
	pairing := createPairing()
	response := httptest.NewRecorder()
	s.ServeHTTP(response, jsonRequest(http.MethodPost, "/auth/pairings/redeem", api.RedeemPairingRequest{Credential: pairing.Credential, Mode: "bearer"}))
	var redeemed api.RedeemPairingResponse
	if response.Code != http.StatusOK || json.NewDecoder(response.Body).Decode(&redeemed) != nil || redeemed.Token == "" || len(response.Result().Cookies()) != 0 {
		t.Fatalf("bearer redemption status = %d, response = %#v", response.Code, redeemed)
	}
	sessionOnly := jsonRequest(http.MethodPost, "/auth/pairings", api.CreatePairingRequest{})
	sessionOnly.Header.Set("Authorization", "Bearer "+redeemed.Token)
	response = httptest.NewRecorder()
	s.ServeHTTP(response, sessionOnly)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("session pairing status = %d", response.Code)
	}
	list := authenticatedRequest(http.MethodGet, "/auth/sessions", "secret")
	response = httptest.NewRecorder()
	s.ServeHTTP(response, list)
	if response.Code != http.StatusOK {
		t.Fatalf("list sessions status = %d", response.Code)
	}
	revoke := authenticatedRequest(http.MethodDelete, "/auth/sessions/"+redeemed.Session.ID, "secret")
	response = httptest.NewRecorder()
	s.ServeHTTP(response, revoke)
	if response.Code != http.StatusOK {
		t.Fatalf("revoke session status = %d", response.Code)
	}

	pairing = createPairing()
	var successes int
	var successesMu sync.Mutex
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response := httptest.NewRecorder()
			s.ServeHTTP(response, jsonRequest(http.MethodPost, "/auth/pairings/redeem", api.RedeemPairingRequest{Credential: pairing.Credential, Mode: "bearer"}))
			if response.Code == http.StatusOK {
				successesMu.Lock()
				successes++
				successesMu.Unlock()
			}
		}()
	}
	wg.Wait()
	if successes != 1 {
		t.Fatalf("concurrent redemption successes = %d", successes)
	}
}

func TestPairingExpiry(t *testing.T) {
	manager := newPairingManager()
	credential, _, err := manager.create(store.DefaultClientID, time.Now().UTC().Add(-pairingLifetime))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := manager.consume(credential, time.Now().UTC()); ok {
		t.Fatal("expired pairing was accepted")
	}
}

func jsonRequest(method, path string, value any) *http.Request {
	body, _ := json.Marshal(value)
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
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
