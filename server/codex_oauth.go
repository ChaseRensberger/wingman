package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/store"
)

const (
	codexClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	codexIssuer   = "https://auth.openai.com"
	codexCallback = "http://localhost:1455/auth/callback"
)

var errOAuthAttemptInactive = errors.New("OAuth attempt is no longer active")

type oauthManager struct {
	store    store.Store
	mu       sync.Mutex
	attempts map[string]*oauthAttempt
}

type oauthAttempt struct {
	id           string
	provider     string
	method       string
	status       string
	url          string
	instructions string
	err          string
	state        string
	verifier     string
	cancel       context.CancelFunc
}

type oauthAttemptDTO struct {
	ID           string `json:"id"`
	Method       string `json:"method"`
	Status       string `json:"status"`
	URL          string `json:"url,omitempty"`
	Instructions string `json:"instructions,omitempty"`
	Error        string `json:"error,omitempty"`
}

func newOAuthManager(data store.Store) *oauthManager {
	return &oauthManager{store: data, attempts: map[string]*oauthAttempt{}}
}

func (m *oauthManager) start(providerID, method string) (oauthAttemptDTO, error) {
	if providerID != "openai" {
		return oauthAttemptDTO{}, fmt.Errorf("OAuth is not supported for provider: %s", providerID)
	}
	if method != "browser" && method != "device" {
		return oauthAttemptDTO{}, fmt.Errorf("unsupported OAuth method: %s", method)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	attempt := &oauthAttempt{id: randomValue(18), provider: providerID, method: method, status: "pending", cancel: cancel}
	m.mu.Lock()
	for _, existing := range m.attempts {
		if existing.provider == providerID && existing.status == "pending" {
			m.mu.Unlock()
			cancel()
			return oauthAttemptDTO{}, fmt.Errorf("an OpenAI OAuth attempt is already in progress")
		}
	}
	m.attempts[attempt.id] = attempt
	m.mu.Unlock()

	var err error
	if method == "browser" {
		err = m.startBrowser(ctx, attempt)
	} else {
		err = m.startDevice(ctx, attempt)
	}
	if err != nil {
		m.finish(attempt.id, "failed", err)
		return oauthAttemptDTO{}, err
	}
	return m.status(attempt.id)
}

func (m *oauthManager) startBrowser(ctx context.Context, attempt *oauthAttempt) error {
	listener, err := net.Listen("tcp", "localhost:1455")
	if err != nil {
		return fmt.Errorf("start OAuth callback listener on localhost:1455: %w", err)
	}
	verifier := randomValue(48)
	challengeSum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeSum[:])
	state := randomValue(32)

	m.mu.Lock()
	attempt.verifier = verifier
	attempt.state = state
	attempt.url = codexAuthorizeURL(codexCallback, challenge, state)
	attempt.instructions = "Complete authorization in your browser. This window will close automatically."
	m.mu.Unlock()

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/callback" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("state") != state {
			m.finish(attempt.id, "failed", fmt.Errorf("invalid OAuth state"))
			writeOAuthPage(w, http.StatusBadRequest, "Authorization failed", "Invalid OAuth state.")
			return
		}
		if message := r.URL.Query().Get("error_description"); message != "" {
			m.finish(attempt.id, "failed", fmt.Errorf("OAuth authorization failed: %s", message))
			writeOAuthPage(w, http.StatusBadRequest, "Authorization failed", message)
			return
		}
		if r.URL.Query().Get("error") != "" {
			m.finish(attempt.id, "failed", fmt.Errorf("OAuth authorization failed"))
			writeOAuthPage(w, http.StatusBadRequest, "Authorization failed", "OpenAI declined authorization.")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			m.finish(attempt.id, "failed", fmt.Errorf("OAuth callback is missing an authorization code"))
			writeOAuthPage(w, http.StatusBadRequest, "Authorization failed", "Missing authorization code.")
			return
		}
		credential, err := exchangeCodexCode(r.Context(), code, codexCallback, verifier)
		if err != nil {
			m.finish(attempt.id, "failed", err)
			writeOAuthPage(w, http.StatusBadRequest, "Authorization failed", "Token exchange failed. Return to Wingman and try again.")
			return
		}
		if err := m.complete(attempt.id, credential); err != nil {
			if errors.Is(err, errOAuthAttemptInactive) {
				writeOAuthPage(w, http.StatusConflict, "Authorization cancelled", "This authorization attempt is no longer active.")
				return
			}
			m.finish(attempt.id, "failed", err)
			writeOAuthPage(w, http.StatusInternalServerError, "Authorization failed", "Wingman could not save the credential.")
			return
		}
		writeOAuthPage(w, http.StatusOK, "OpenAI connected", "You can close this window and return to Wingman.")
	})
	server := &http.Server{Handler: h}
	go func() { _ = server.Serve(listener) }()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		if ctx.Err() == context.DeadlineExceeded {
			m.finish(attempt.id, "failed", fmt.Errorf("OAuth authorization timed out"))
		}
	}()
	return nil
}

func (m *oauthManager) startDevice(ctx context.Context, attempt *oauthAttempt) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, codexIssuer+"/api/accounts/deviceauth/usercode", strings.NewReader(`{"client_id":"`+codexClientID+`"}`))
	if err != nil {
		return err
	}
	request.Header.Set("content-type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("start OpenAI device authorization: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("start OpenAI device authorization: HTTP %d", response.StatusCode)
	}
	var device struct {
		DeviceAuthID string `json:"device_auth_id"`
		UserCode     string `json:"user_code"`
		Interval     string `json:"interval"`
	}
	if err := json.NewDecoder(response.Body).Decode(&device); err != nil {
		return fmt.Errorf("decode OpenAI device authorization: %w", err)
	}
	if device.DeviceAuthID == "" || device.UserCode == "" {
		return fmt.Errorf("OpenAI device authorization returned incomplete data")
	}
	interval, _ := time.ParseDuration(device.Interval + "s")
	if interval < time.Second {
		interval = 5 * time.Second
	}
	m.mu.Lock()
	attempt.url = codexIssuer + "/codex/device"
	attempt.instructions = "Enter code: " + device.UserCode
	m.mu.Unlock()
	go m.pollDevice(ctx, attempt.id, device.DeviceAuthID, device.UserCode, interval)
	return nil
}

func (m *oauthManager) pollDevice(ctx context.Context, attemptID, deviceAuthID, userCode string, interval time.Duration) {
	for {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				m.finish(attemptID, "failed", fmt.Errorf("OAuth authorization timed out"))
			}
			return
		case <-time.After(interval + 3*time.Second):
		}
		body := fmt.Sprintf(`{"device_auth_id":%q,"user_code":%q}`, deviceAuthID, userCode)
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, codexIssuer+"/api/accounts/deviceauth/token", strings.NewReader(body))
		if err != nil {
			m.finish(attemptID, "failed", err)
			return
		}
		request.Header.Set("content-type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			m.finish(attemptID, "failed", fmt.Errorf("poll OpenAI device authorization: %w", err))
			return
		}
		if response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusNotFound {
			response.Body.Close()
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			response.Body.Close()
			m.finish(attemptID, "failed", fmt.Errorf("poll OpenAI device authorization: HTTP %d", response.StatusCode))
			return
		}
		var authorization struct {
			Code         string `json:"authorization_code"`
			CodeVerifier string `json:"code_verifier"`
		}
		err = json.NewDecoder(response.Body).Decode(&authorization)
		response.Body.Close()
		if err != nil {
			m.finish(attemptID, "failed", fmt.Errorf("decode OpenAI device authorization: %w", err))
			return
		}
		credential, err := exchangeCodexCode(ctx, authorization.Code, codexIssuer+"/deviceauth/callback", authorization.CodeVerifier)
		if err != nil {
			m.finish(attemptID, "failed", err)
			return
		}
		if err := m.complete(attemptID, credential); err != nil {
			if errors.Is(err, errOAuthAttemptInactive) {
				return
			}
			m.finish(attemptID, "failed", err)
			return
		}
		return
	}
}

func (m *oauthManager) refresh(ctx context.Context, providerID string, stale provider.Credential) (provider.Credential, error) {
	if providerID != "openai" {
		return provider.Credential{}, fmt.Errorf("OAuth refresh is not supported for provider: %s", providerID)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	auth, err := m.store.GetAuth()
	if err != nil {
		return provider.Credential{}, err
	}
	cred, ok := auth.Providers[providerID]
	if !ok || cred.Type != "oauth" || cred.Refresh == "" {
		return provider.Credential{}, fmt.Errorf("OpenAI OAuth credential is missing; reconnect the provider")
	}
	if cred.Access != "" && cred.ExpiresAt > time.Now().Unix() {
		return providerCredential(cred), nil
	}
	fresh, err := refreshCodexCredential(ctx, cred)
	if err != nil {
		return provider.Credential{}, err
	}
	auth.Providers[providerID] = fresh
	if err := m.store.SetAuth(auth); err != nil {
		return provider.Credential{}, fmt.Errorf("save refreshed OpenAI OAuth credential: %w", err)
	}
	return providerCredential(fresh), nil
}

func (m *oauthManager) complete(id string, credential store.AuthCredential) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	attempt, ok := m.attempts[id]
	if !ok || attempt.status != "pending" {
		return errOAuthAttemptInactive
	}
	auth, err := m.store.GetAuth()
	if err != nil {
		return err
	}
	auth.Providers["openai"] = credential
	if err := m.store.SetAuth(auth); err != nil {
		return err
	}
	attempt.status = "completed"
	attempt.cancel()
	return nil
}

func (m *oauthManager) status(id string) (oauthAttemptDTO, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	attempt, ok := m.attempts[id]
	if !ok {
		return oauthAttemptDTO{}, fmt.Errorf("OAuth attempt not found")
	}
	return oauthAttemptDTO{ID: attempt.id, Method: attempt.method, Status: attempt.status, URL: attempt.url, Instructions: attempt.instructions, Error: attempt.err}, nil
}

func (m *oauthManager) cancel(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	attempt, ok := m.attempts[id]
	if !ok {
		return fmt.Errorf("OAuth attempt not found")
	}
	if attempt.status == "pending" {
		attempt.status = "cancelled"
		attempt.cancel()
	}
	return nil
}

func (m *oauthManager) finish(id, status string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	attempt, ok := m.attempts[id]
	if !ok || attempt.status != "pending" {
		return
	}
	attempt.status = status
	if err != nil {
		attempt.err = err.Error()
	}
	attempt.cancel()
}

func (s *Server) refreshProviderCredential(ctx context.Context, providerID string, stale provider.Credential) (provider.Credential, error) {
	return s.oauth.refresh(ctx, providerID, stale)
}

func (s *Server) handleProviderOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	var request struct {
		Method string `json:"method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	attempt, err := s.oauth.start(chi.URLParam(r, "name"), request.Method)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, attempt)
}

func (s *Server) handleProviderOAuthStatus(w http.ResponseWriter, r *http.Request) {
	attempt, err := s.oauth.status(chi.URLParam(r, "attempt"))
	if err != nil || chi.URLParam(r, "name") != "openai" {
		writeError(w, http.StatusNotFound, "OAuth attempt not found")
		return
	}
	writeJSON(w, http.StatusOK, attempt)
}

func (s *Server) handleProviderOAuthCancel(w http.ResponseWriter, r *http.Request) {
	if err := s.oauth.cancel(chi.URLParam(r, "attempt")); err != nil {
		writeError(w, http.StatusNotFound, "OAuth attempt not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func codexAuthorizeURL(redirectURI, challenge, state string) string {
	params := url.Values{
		"response_type":              {"code"},
		"client_id":                  {codexClientID},
		"redirect_uri":               {redirectURI},
		"scope":                      {"openid profile email offline_access"},
		"code_challenge":             {challenge},
		"code_challenge_method":      {"S256"},
		"id_token_add_organizations": {"true"},
		"codex_cli_simplified_flow":  {"true"},
		"state":                      {state},
		"originator":                 {"codex_cli_rs"},
	}
	return codexIssuer + "/oauth/authorize?" + params.Encode()
}

func exchangeCodexCode(ctx context.Context, code, redirectURI, verifier string) (store.AuthCredential, error) {
	values := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirectURI}, "client_id": {codexClientID}, "code_verifier": {verifier}}
	return codexTokenRequest(ctx, values, store.AuthCredential{Type: "oauth"})
}

func refreshCodexCredential(ctx context.Context, credential store.AuthCredential) (store.AuthCredential, error) {
	values := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {credential.Refresh}, "client_id": {codexClientID}}
	return codexTokenRequest(ctx, values, credential)
}

func codexTokenRequest(ctx context.Context, values url.Values, prior store.AuthCredential) (store.AuthCredential, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, codexIssuer+"/oauth/token", strings.NewReader(values.Encode()))
	if err != nil {
		return store.AuthCredential{}, err
	}
	request.Header.Set("content-type", "application/x-www-form-urlencoded")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return store.AuthCredential{}, fmt.Errorf("request OpenAI OAuth token: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
		return store.AuthCredential{}, fmt.Errorf("OpenAI OAuth token request: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var tokens struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(response.Body).Decode(&tokens); err != nil {
		return store.AuthCredential{}, fmt.Errorf("decode OpenAI OAuth token response: %w", err)
	}
	if tokens.AccessToken == "" {
		return store.AuthCredential{}, fmt.Errorf("OpenAI OAuth token response is missing an access token")
	}
	if tokens.RefreshToken == "" {
		tokens.RefreshToken = prior.Refresh
	}
	if tokens.ExpiresIn == 0 {
		tokens.ExpiresIn = 3600
	}
	accountID := codexAccountID(tokens.IDToken)
	if accountID == "" {
		accountID = codexAccountID(tokens.AccessToken)
	}
	if accountID == "" {
		accountID = prior.AccountID
	}
	return store.AuthCredential{Type: "oauth", Access: tokens.AccessToken, Refresh: tokens.RefreshToken, ExpiresAt: time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second).Unix(), AccountID: accountID}, nil
}

func providerCredential(credential store.AuthCredential) provider.Credential {
	return provider.Credential{Type: credential.Type, Key: credential.Key, Access: credential.Access, Refresh: credential.Refresh, ExpiresAt: credential.ExpiresAt, AccountID: credential.AccountID}
}

func codexAccountID(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		AccountID     string `json:"chatgpt_account_id"`
		Organizations []struct {
			ID string `json:"id"`
		} `json:"organizations"`
		Auth struct {
			AccountID string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}
	if claims.AccountID != "" {
		return claims.AccountID
	}
	if claims.Auth.AccountID != "" {
		return claims.Auth.AccountID
	}
	if len(claims.Organizations) > 0 {
		return claims.Organizations[0].ID
	}
	return ""
}

func randomValue(size int) string {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		panic(fmt.Sprintf("generate OAuth random value: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(bytes)
}

func writeOAuthPage(w http.ResponseWriter, status int, title, message string) {
	w.Header().Set("content-type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "<!doctype html><title>%s</title><main><h1>%s</h1><p>%s</p></main>", html.EscapeString(title), html.EscapeString(title), html.EscapeString(message))
}
