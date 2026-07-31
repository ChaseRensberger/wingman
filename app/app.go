// Package app owns one Wingman daemon instance and its process-lifetime resources.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/chaserensberger/wingman/internal/observability"
	wingmcp "github.com/chaserensberger/wingman/mcp"
	provider "github.com/chaserensberger/wingman/models/providers"
	_ "github.com/chaserensberger/wingman/models/providers/anthropic"
	_ "github.com/chaserensberger/wingman/models/providers/deepseek"
	_ "github.com/chaserensberger/wingman/models/providers/google"
	_ "github.com/chaserensberger/wingman/models/providers/openai"
	_ "github.com/chaserensberger/wingman/models/providers/openaicompat"
	_ "github.com/chaserensberger/wingman/models/providers/opencode"
	_ "github.com/chaserensberger/wingman/models/providers/opencodego"
	_ "github.com/chaserensberger/wingman/models/providers/openrouter"
	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/pluginhost"
	"github.com/chaserensberger/wingman/server"
	"github.com/chaserensberger/wingman/store"
)

const defaultShutdownTimeout = 30 * time.Second

// Config describes one daemon instance without preconstructing owned resources.
type Config struct {
	Ephemeral         bool
	DBPath            string
	WebDevURL         string
	LogFormat         string
	LogLevel          string
	LogWriter         io.Writer
	Logger            *slog.Logger
	PluginDirs        []string
	DefaultPluginDir  string
	DisablePlugins    bool
	MCP               map[string]wingmcp.ServerConfig
	Providers         map[string]provider.ProviderConfig
	Permissions       permission.Ruleset
	AgentPermissions  map[string]permission.Ruleset
	PermissionTimeout time.Duration
	ShutdownTimeout   time.Duration
}

type lifecycleServer interface {
	http.Handler
	Start(context.Context) error
	Close(context.Context) error
}

type storeResource struct {
	store store.Store
	close func() error
}

type pluginResource struct {
	manager *pluginhost.Manager
	close   func() error
}

type mcpResource struct {
	manager *wingmcp.Manager
	close   func() error
}

type factories struct {
	openStore  func(string) (storeResource, error)
	newPlugins func(context.Context, []string) (pluginResource, error)
	newMCP     func(context.Context, map[string]wingmcp.ServerConfig) (mcpResource, error)
	newServer  func(server.Config) lifecycleServer
}

// App is the lifecycle root for one Wingman daemon.
type App struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Config

	server  lifecycleServer
	logger  *slog.Logger
	logs    *observability.LogBuffer
	store   storeResource
	plugins pluginResource
	mcp     mcpResource

	closeMu   sync.Mutex
	closing   bool
	closed    bool
	serveMu   sync.Mutex
	serveDone chan struct{}
}

// New constructs and starts one daemon. Partial construction is rolled back in
// reverse ownership order.
func New(ctx context.Context, cfg Config) (*App, error) {
	return newWithFactories(ctx, cfg, defaultFactories())
}

func newWithFactories(ctx context.Context, cfg Config, f factories) (*App, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	root, cancel := context.WithCancel(ctx)
	a := &App{ctx: root, cancel: cancel, cfg: cfg}
	var rollback []func() error
	fail := func(err error) (*App, error) {
		cancel()
		var rollbackErrs []error
		for i := len(rollback) - 1; i >= 0; i-- {
			rollbackErrs = append(rollbackErrs, rollback[i]())
		}
		return nil, errors.Join(err, errors.Join(rollbackErrs...))
	}

	a.logs = observability.NewLogBuffer(500)
	a.logger = cfg.Logger
	if a.logger == nil {
		var err error
		a.logger, err = observability.NewBufferedLogger(cfg.LogWriter, cfg.LogFormat, cfg.LogLevel, a.logs)
		if err != nil {
			return fail(fmt.Errorf("initialize logging: %w", err))
		}
	}

	if !cfg.Ephemeral {
		path := cfg.DBPath
		if path == "" {
			var err error
			path, err = store.DefaultDBPath()
			if err != nil {
				return fail(fmt.Errorf("resolve database path: %w", err))
			}
		}
		resource, err := f.openStore(path)
		if err != nil {
			return fail(fmt.Errorf("initialize storage: %w", err))
		}
		a.store = resource
		rollback = append(rollback, resource.close)
	}

	if !cfg.DisablePlugins {
		dir := cfg.DefaultPluginDir
		if dir == "" {
			var err error
			dir, err = pluginhost.DefaultGlobalDir()
			if err != nil {
				return fail(fmt.Errorf("resolve default plugin directory: %w", err))
			}
		}
		dirs := append([]string{dir}, cfg.PluginDirs...)
		resource, err := f.newPlugins(root, dirs)
		if err != nil {
			return fail(fmt.Errorf("initialize plugins: %w", err))
		}
		a.plugins = resource
		rollback = append(rollback, resource.close)
	}

	mcpResource, err := f.newMCP(root, cfg.MCP)
	if err != nil {
		return fail(fmt.Errorf("initialize MCP: %w", err))
	}
	a.mcp = mcpResource
	rollback = append(rollback, mcpResource.close)

	// Config overlays remain process-global until Wave C2 introduces immutable
	// application-owned provider and catalog generations.
	provider.RegisterConfig(cfg.Providers)
	a.server = f.newServer(server.Config{
		RootContext: root, Store: a.store.store, WebDevURL: cfg.WebDevURL,
		Logger: a.logger, Logs: a.logs, Plugins: a.plugins.manager, MCP: a.mcp.manager,
		Providers: cfg.Providers, Permissions: cfg.Permissions,
		AgentPermissions: cfg.AgentPermissions, PermissionTimeout: cfg.PermissionTimeout,
	})
	rollback = append(rollback, func() error { return a.server.Close(context.Background()) })
	if err := a.server.Start(ctx); err != nil {
		return fail(fmt.Errorf("start application: %w", err))
	}
	return a, nil
}

// Handler returns the daemon HTTP adapter.
func (a *App) Handler() http.Handler { return a.server }

// Logger returns the application-owned structured logger.
func (a *App) Logger() *slog.Logger { return a.logger }

// Serve serves the application on listener until ctx ends, the application is
// closed, or the listener fails. It drains HTTP before closing dependencies.
func (a *App) Serve(ctx context.Context, listener net.Listener) error {
	a.serveMu.Lock()
	if a.serveDone != nil {
		a.serveMu.Unlock()
		return fmt.Errorf("application is already serving")
	}
	a.closeMu.Lock()
	closing := a.closing || a.closed || a.ctx.Err() != nil
	a.closeMu.Unlock()
	if closing {
		a.serveMu.Unlock()
		return fmt.Errorf("application is closing")
	}
	serveDone := make(chan struct{})
	a.serveDone = serveDone
	a.serveMu.Unlock()
	defer func() {
		a.serveMu.Lock()
		close(serveDone)
		a.serveDone = nil
		a.serveMu.Unlock()
	}()

	httpServer := &http.Server{Handler: a.Handler()}
	serveErr := make(chan error, 1)
	go func() { serveErr <- httpServer.Serve(listener) }()

	var triggerErr error
	select {
	case err := <-serveErr:
		if !isServerClosed(err) {
			triggerErr = err
		}
	case <-ctx.Done():
		triggerErr = ctx.Err()
	case <-a.ctx.Done():
	}

	timeout := a.cfg.ShutdownTimeout
	if timeout <= 0 {
		timeout = defaultShutdownTimeout
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	a.closeMu.Lock()
	a.closing = true
	a.closeMu.Unlock()
	a.cancel()
	_ = listener.Close()
	httpErr := httpServer.Shutdown(shutdownCtx)
	if httpErr != nil {
		httpErr = errors.Join(httpErr, httpServer.Close())
	}
	closeErr := a.close(shutdownCtx)
	select {
	case err := <-serveErr:
		if triggerErr == nil && !isServerClosed(err) {
			triggerErr = err
		}
	default:
	}
	if errors.Is(triggerErr, context.Canceled) {
		triggerErr = nil
	}
	return errors.Join(triggerErr, httpErr, closeErr)
}

func isServerClosed(err error) bool {
	return errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed)
}

// Close stops application work, then closes MCP, plugins, and storage in
// dependency order. If server work does not drain before ctx ends, dependencies
// remain open and Close may be retried.
func (a *App) Close(ctx context.Context) error {
	a.serveMu.Lock()
	a.closeMu.Lock()
	a.closing = true
	a.closeMu.Unlock()
	a.cancel()
	serveDone := a.serveDone
	if serveDone == nil {
		defer a.serveMu.Unlock()
		return a.close(ctx)
	}
	a.serveMu.Unlock()
	select {
	case <-serveDone:
	case <-ctx.Done():
		return ctx.Err()
	}
	return a.close(ctx)
}

func (a *App) close(ctx context.Context) error {
	a.closeMu.Lock()
	defer a.closeMu.Unlock()
	if a.closed {
		return nil
	}
	if err := a.server.Close(ctx); err != nil {
		return err
	}
	var errs []error
	if a.mcp.close != nil {
		errs = append(errs, a.mcp.close())
	}
	if a.plugins.close != nil {
		errs = append(errs, a.plugins.close())
	}
	if a.store.close != nil {
		errs = append(errs, a.store.close())
	}
	a.closed = true
	return errors.Join(errs...)
}

func defaultFactories() factories {
	return factories{
		openStore: func(path string) (storeResource, error) {
			value, err := store.NewSQLiteStore(path)
			if err != nil {
				return storeResource{}, err
			}
			return storeResource{store: value, close: value.Close}, nil
		},
		newPlugins: func(ctx context.Context, dirs []string) (pluginResource, error) {
			value, err := pluginhost.New(ctx, dirs)
			if err != nil {
				return pluginResource{}, err
			}
			return pluginResource{manager: value, close: value.Close}, nil
		},
		newMCP: func(ctx context.Context, servers map[string]wingmcp.ServerConfig) (mcpResource, error) {
			value := wingmcp.New(ctx, wingmcp.Config{Servers: servers})
			return mcpResource{manager: value, close: value.Close}, nil
		},
		newServer: func(cfg server.Config) lifecycleServer { return server.New(cfg) },
	}
}
