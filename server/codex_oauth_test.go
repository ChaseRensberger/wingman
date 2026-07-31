package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestCodexAccountID(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"https://api.openai.com/auth": map[string]string{"chatgpt_account_id": "acc_test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	token := "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
	if got := codexAccountID(token); got != "acc_test" {
		t.Fatalf("account ID = %q", got)
	}
}

func TestOAuthCompleteDoesNotPersistCancelledAttempt(t *testing.T) {
	data := memory.NewStore()
	auth, err := data.GetAuth()
	if err != nil {
		t.Fatal(err)
	}
	auth.Providers["openai"] = store.AuthCredential{Type: "api_key", Key: "existing-key"}
	if err := data.SetAuth(auth); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := newOAuthManager(context.Background(), data)
	manager.attempts["attempt"] = &oauthAttempt{id: "attempt", provider: "openai", status: "pending", cancel: cancel}
	if err := manager.cancel("attempt"); err != nil {
		t.Fatal(err)
	}
	if err := manager.complete("attempt", store.AuthCredential{Type: "oauth", Access: "new-access", Refresh: "new-refresh"}); !errors.Is(err, errOAuthAttemptInactive) {
		t.Fatalf("complete error = %v, want inactive attempt", err)
	}
	if ctx.Err() == nil {
		t.Fatal("cancel did not cancel the attempt context")
	}

	auth, err = data.GetAuth()
	if err != nil {
		t.Fatal(err)
	}
	if got := auth.Providers["openai"]; got.Type != "api_key" || got.Key != "existing-key" {
		t.Fatalf("credential = %#v, want original API key", got)
	}
}

func TestOAuthCompletePersistsPendingAttempt(t *testing.T) {
	data := memory.NewStore()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := newOAuthManager(context.Background(), data)
	manager.attempts["attempt"] = &oauthAttempt{id: "attempt", provider: "openai", status: "pending", cancel: cancel}
	credential := store.AuthCredential{Type: "oauth", Access: "access", Refresh: "refresh", AccountID: "account"}
	if err := manager.complete("attempt", credential); err != nil {
		t.Fatal(err)
	}

	attempt, err := manager.status("attempt")
	if err != nil {
		t.Fatal(err)
	}
	if attempt.Status != "completed" {
		t.Fatalf("status = %q, want completed", attempt.Status)
	}
	auth, err := data.GetAuth()
	if err != nil {
		t.Fatal(err)
	}
	if got := auth.Providers["openai"]; got != credential {
		t.Fatalf("credential = %#v, want %#v", got, credential)
	}
}

func TestCodexAuthorizeURL(t *testing.T) {
	raw := codexAuthorizeURL(codexCallback, "challenge", "state")
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Host != "auth.openai.com" || parsed.Path != "/oauth/authorize" {
		t.Fatalf("URL = %s", raw)
	}
	query := parsed.Query()
	if query.Get("client_id") != codexClientID || query.Get("redirect_uri") != codexCallback {
		t.Fatalf("query = %v", query)
	}
	if query.Get("originator") != "codex_cli_rs" || query.Get("code_challenge_method") != "S256" {
		t.Fatalf("query = %v", query)
	}
}

func TestOAuthCloseRejectsStartsAndWaitsForWorkers(t *testing.T) {
	manager := newOAuthManager(context.Background(), memory.NewStore())
	finished := make(chan struct{})
	if !manager.startWorker(func() { <-finished }) {
		t.Fatal("start worker")
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := manager.Close(timeoutCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("close error = %v, want deadline exceeded", err)
	}
	if _, err := manager.start("openai", "device"); err == nil {
		t.Fatal("start after close succeeded")
	}
	close(finished)
	if err := manager.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
