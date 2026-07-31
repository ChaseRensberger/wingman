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

	"github.com/chaserensberger/wingman/execution"
	wingmcp "github.com/chaserensberger/wingman/mcp"
	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
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
	wantErr := errors.New("scope failed")
	_, err := newWithFactories(context.Background(), Config{}, factories{
		openStore: func(string) (storeResource, error) {
			return storeResource{close: func() error { order = append(order, "store"); return nil }}, nil
		},
		newScopes: func(execution.Config) (scopeResource, error) {
			return scopeResource{}, wantErr
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("New() error = %v", err)
	}
	if want := []string{"store"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("rollback order = %v, want %v", order, want)
	}
}

func TestNewValidatesProviderGenerationBeforeOpeningStore(t *testing.T) {
	opened := false
	_, err := newWithFactories(context.Background(), Config{Providers: map[string]provider.ProviderConfig{
		"invalid": {Options: provider.ProviderOptions{BaseURL: "https://example.test"}, Models: map[string]models.ModelInfo{
			"model": {API: "unsupported"},
		}},
	}}, factories{
		openStore: func(string) (storeResource, error) {
			opened = true
			return storeResource{}, nil
		},
	})
	if err == nil {
		t.Fatal("New accepted invalid provider config")
	}
	if opened {
		t.Fatal("store opened before provider validation")
	}
}

func TestNewValidatesMCPBeforeOpeningStore(t *testing.T) {
	opened := false
	_, err := newWithFactories(context.Background(), Config{MCP: map[string]wingmcp.ServerConfig{
		"invalid": {Type: "remote", URL: "relative"},
	}}, factories{
		openStore: func(string) (storeResource, error) {
			opened = true
			return storeResource{}, nil
		},
	})
	if err == nil {
		t.Fatal("New accepted invalid MCP config")
	}
	if opened {
		t.Fatal("store opened before MCP validation")
	}
}

func TestNewRollsBackAllResourcesWhenServerStartFails(t *testing.T) {
	wantErr := errors.New("recovery failed")
	var order []string
	_, err := newWithFactories(context.Background(), Config{}, factories{
		openStore: func(string) (storeResource, error) {
			return storeResource{close: func() error { order = append(order, "store"); return nil }}, nil
		},
		newScopes: func(execution.Config) (scopeResource, error) {
			return scopeResource{close: func(context.Context) error { order = append(order, "scopes"); return nil }}, nil
		},
		newServer: func(server.Config) lifecycleServer {
			return &orderedFakeServer{fakeServer: fakeServer{startErr: wantErr}, order: &order}
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("New() error = %v", err)
	}
	if want := []string{"server", "scopes", "store"}; !reflect.DeepEqual(order, want) {
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
		scopes: scopeResource{close: func(context.Context) error { order = append(order, "scopes"); return nil }},
		store:  storeResource{close: func() error { order = append(order, "store"); return nil }},
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
	if want := []string{"scopes", "store"}; !reflect.DeepEqual(order, want) {
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

func TestApplicationsUseIsolatedProviderGenerations(t *testing.T) {
	configured, err := New(context.Background(), Config{
		Ephemeral: true, DisablePlugins: true,
		Providers: map[string]provider.ProviderConfig{
			"custom": {
				Name: "Custom", Options: provider.ProviderOptions{BaseURL: "https://example.test/v1"},
				Models: map[string]models.ModelInfo{"chat": {API: models.APIOpenAICompatible}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	configured.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/provider/custom/models", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("configured status = %d, body = %s", response.Code, response.Body.String())
	}
	if err := configured.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	clean, err := New(context.Background(), Config{Ephemeral: true, DisablePlugins: true})
	if err != nil {
		t.Fatal(err)
	}
	defer clean.Close(context.Background())
	response = httptest.NewRecorder()
	clean.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/provider/custom/models", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("clean status = %d, body = %s", response.Code, response.Body.String())
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
