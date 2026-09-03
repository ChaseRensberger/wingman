package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestStartRecoversOnlyOnce(t *testing.T) {
	recoveryStore := &startupRecoveryStore{Store: memory.NewStore()}
	server := New(Config{Store: recoveryStore})
	server.runs.reconcileInterval = time.Hour
	t.Cleanup(func() { _ = server.Close(context.Background()) })

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- server.Start(context.Background())
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got, want := len(recoveryStore.order), 4; got != want {
		t.Fatalf("recovery calls = %d, want %d (%v)", got, want, recoveryStore.order)
	}
}

func TestReadinessIdentifiesFailedRecoverySubsystem(t *testing.T) {
	server := New(Config{Store: &startupRecoveryStore{Store: memory.NewStore(), permissionErr: errors.New("database unavailable")}, Password: "secret"})
	t.Cleanup(func() { _ = server.Close(context.Background()) })
	if err := server.Start(context.Background()); err == nil {
		t.Fatal("Start succeeded, want recovery failure")
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/ready", "secret"))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var readiness api.ReadinessResponse
	if err := json.NewDecoder(response.Body).Decode(&readiness); err != nil {
		t.Fatal(err)
	}
	if readiness.Diagnostic == nil || readiness.Diagnostic.Subsystem != "permission_recovery" {
		t.Fatalf("readiness = %#v", readiness)
	}
}

func TestManagedServiceRestartRequest(t *testing.T) {
	restarted := make(chan struct{}, 1)
	server := New(Config{RequestRestart: func() { restarted <- struct{}{} }})

	request := httptest.NewRequest(http.MethodPost, "/service/restart", nil)
	request.Header.Set("X-Wingman-Console", "1")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || response.Body.String() != "{\"status\":\"restarting\"}\n" {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
	select {
	case <-restarted:
	case <-time.After(time.Second):
		t.Fatal("restart callback was not called")
	}
}

func TestServiceRestartRequiresManagedDaemonAndConsoleHeader(t *testing.T) {
	tests := []struct {
		name   string
		server *Server
		header string
		status int
	}{
		{name: "foreground daemon", server: New(Config{}), header: "1", status: http.StatusConflict},
		{name: "missing console header", server: New(Config{RequestRestart: func() {}}), status: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/service/restart", nil)
			if test.header != "" {
				request.Header.Set("X-Wingman-Console", test.header)
			}
			response := httptest.NewRecorder()
			test.server.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestCloseIsConcurrentAndRetryable(t *testing.T) {
	server := New(Config{})
	done := server.trackInflight()

	timeoutCtx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := server.Close(timeoutCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first close error = %v, want deadline exceeded", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- server.Close(context.Background())
		}()
	}
	time.Sleep(time.Millisecond)
	done()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent close error = %v", err)
		}
	}
}

func TestRootCancellationClosesServer(t *testing.T) {
	root, cancel := context.WithCancel(context.Background())
	server := New(Config{RootContext: root})
	cancel()

	select {
	case <-server.ShutdownCtx().Done():
	case <-time.After(time.Second):
		t.Fatal("root cancellation did not cancel server")
	}
	if err := server.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCloseCancelsPermissionWait(t *testing.T) {
	data := memory.NewStore()
	if err := data.CreateSession(&store.Session{ID: "ses_close_permission"}); err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: data})
	result := make(chan error, 1)
	go func() {
		_, err := server.permissionRequests.prompter("ses_close_permission", "").Request(context.Background(), run.PermissionRequestInfo{Action: "edit", Resources: []string{"a.go"}})
		result <- err
	}()
	waitForPermissionRequest(t, data, "ses_close_permission")
	if err := server.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("permission error = %v, want canceled", err)
	}
}
