package server

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
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
