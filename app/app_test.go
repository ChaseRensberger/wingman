package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	wingmcp "github.com/chaserensberger/wingman/mcp"
	"github.com/chaserensberger/wingman/server"
)

type fakeServer struct {
	startErr error
	closeErr error
	closed   int
	handler  http.Handler
}

func (s *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.handler != nil {
		s.handler.ServeHTTP(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *fakeServer) Start(context.Context) error { return s.startErr }
func (s *fakeServer) Close(context.Context) error {
	s.closed++
	err := s.closeErr
	s.closeErr = nil
	return err
}

func TestNewRollsBackResourcesInReverseOrder(t *testing.T) {
	var order []string
	wantErr := errors.New("MCP failed")
	_, err := newWithFactories(context.Background(), Config{}, factories{
		openStore: func(string) (storeResource, error) {
			return storeResource{close: func() error { order = append(order, "store"); return nil }}, nil
		},
		newPlugins: func(context.Context, []string) (pluginResource, error) {
			return pluginResource{close: func() error { order = append(order, "plugins"); return nil }}, nil
		},
		newMCP: func(context.Context, map[string]wingmcp.ServerConfig) (mcpResource, error) {
			return mcpResource{}, wantErr
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("New() error = %v", err)
	}
	if want := []string{"plugins", "store"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("rollback order = %v, want %v", order, want)
	}
}

func TestNewRollsBackAllResourcesWhenServerStartFails(t *testing.T) {
	wantErr := errors.New("recovery failed")
	var order []string
	_, err := newWithFactories(context.Background(), Config{}, factories{
		openStore: func(string) (storeResource, error) {
			return storeResource{close: func() error { order = append(order, "store"); return nil }}, nil
		},
		newPlugins: func(context.Context, []string) (pluginResource, error) {
			return pluginResource{close: func() error { order = append(order, "plugins"); return nil }}, nil
		},
		newMCP: func(context.Context, map[string]wingmcp.ServerConfig) (mcpResource, error) {
			return mcpResource{close: func() error { order = append(order, "mcp"); return nil }}, nil
		},
		newServer: func(server.Config) lifecycleServer {
			return &orderedFakeServer{fakeServer: fakeServer{startErr: wantErr}, order: &order}
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("New() error = %v", err)
	}
	if want := []string{"server", "mcp", "plugins", "store"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("rollback order = %v, want %v", order, want)
	}
}

type orderedFakeServer struct {
	fakeServer
	order *[]string
}

func (s *orderedFakeServer) Close(ctx context.Context) error {
	*s.order = append(*s.order, "server")
	return s.fakeServer.Close(ctx)
}

func TestCloseWaitsForServerBeforeDependenciesAndCanRetry(t *testing.T) {
	deadline := context.DeadlineExceeded
	core := &fakeServer{closeErr: deadline}
	var order []string
	a := &App{
		cancel: func() {}, server: core,
		mcp:     mcpResource{close: func() error { order = append(order, "mcp"); return nil }},
		plugins: pluginResource{close: func() error { order = append(order, "plugins"); return nil }},
		store:   storeResource{close: func() error { order = append(order, "store"); return nil }},
	}
	if err := a.Close(context.Background()); !errors.Is(err, deadline) {
		t.Fatalf("first Close() error = %v", err)
	}
	if len(order) != 0 {
		t.Fatalf("dependencies closed before server drained: %v", order)
	}
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := []string{"mcp", "plugins", "store"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("close order = %v, want %v", order, want)
	}
	if err := a.Close(context.Background()); err != nil || core.closed != 2 {
		t.Fatalf("idempotent Close() = %v, server closes = %d", err, core.closed)
	}
}

func TestHandlerDelegatesToServer(t *testing.T) {
	a := &App{server: &fakeServer{}}
	response := httptest.NewRecorder()
	a.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestNewEphemeralApplication(t *testing.T) {
	a, err := New(context.Background(), Config{Ephemeral: true, DisablePlugins: true, ShutdownTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestServeStopsOnContextCancellation(t *testing.T) {
	a, err := New(context.Background(), Config{Ephemeral: true, DisablePlugins: true, ShutdownTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Serve(ctx, listener) }()
	response, err := http.Get("http://" + listener.Addr().String() + "/health")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not stop")
	}
}

func TestServeDrainsHTTPBeforeClosingDependencies(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	dependencyClosed := make(chan struct{})
	root, rootCancel := context.WithCancel(context.Background())
	core := &fakeServer{handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	})}
	a := &App{
		ctx: root, cancel: rootCancel, cfg: Config{ShutdownTimeout: time.Second}, server: core,
		store: storeResource{close: func() error { close(dependencyClosed); return nil }},
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- a.Serve(context.Background(), listener) }()
	requestDone := make(chan error, 1)
	go func() {
		response, err := http.Get("http://" + listener.Addr().String())
		if err == nil {
			_ = response.Body.Close()
		}
		requestDone <- err
	}()
	<-started
	closeDone := make(chan error, 1)
	go func() { closeDone <- a.Close(context.Background()) }()
	select {
	case <-dependencyClosed:
		t.Fatal("dependency closed before HTTP request drained")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	if err := <-requestDone; err != nil {
		t.Fatal(err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	if err := <-serveDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-dependencyClosed:
	default:
		t.Fatal("dependency did not close")
	}
}

var _ lifecycleServer = (*server.Server)(nil)
