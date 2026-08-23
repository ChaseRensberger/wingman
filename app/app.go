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
	"path/filepath"
	"sync"
	"time"

	"github.com/chaserensberger/wingman/execution"
	"github.com/chaserensberger/wingman/internal/daemonstate"
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
	ConsoleDevURL     string
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
	Password          string
	Username          string
	InstanceID        string
	Version           string
	// GlobalInstructionsPath selects the daemon-wide AGENTS.md file. An empty
	// path uses the Wingman configuration directory.
	GlobalInstructionsPath string
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

type scopeResource struct {
	manager *execution.Manager
	close   func(context.Context) error
}

type factories struct {
	openStore func(string) (storeResource, error)
	newScopes func(execution.Config) (scopeResource, error)
	newServer func(server.Config) lifecycleServer
}

// App is the lifecycle root for one Wingman daemon.
type App struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Config

	server lifecycleServer
	logger *slog.Logger
	logs   *observability.LogBuffer
	store  storeResource
	scopes scopeResource

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
	providers, err := provider.NewRegistry(cfg.Providers)
	if err != nil {
		return fail(fmt.Errorf("initialize provider registry: %w", err))
	}
	if err := (wingmcp.Config{Servers: cfg.MCP}).Validate(); err != nil {
		return fail(fmt.Errorf("validate MCP config: %w", err))
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

	dirs := append([]string(nil), cfg.PluginDirs...)
	if !cfg.DisablePlugins {
		defaultDir := cfg.DefaultPluginDir
		if defaultDir == "" {
			defaultDir, err = pluginhost.DefaultGlobalDir()
			if err != nil {
				return fail(fmt.Errorf("resolve default plugin directory: %w", err))
			}
		}
		dirs = append([]string{defaultDir}, dirs...)
	}
	a.scopes, err = f.newScopes(execution.Config{
		RootContext: root, PluginDirs: dirs, DisablePlugins: cfg.DisablePlugins,
		MCP: cfg.MCP, Providers: providers, NativeTools: execution.BuiltinTools(),
	})
	if err != nil {
		return fail(fmt.Errorf("initialize execution scopes: %w", err))
	}
	rollback = append(rollback, func() error { return a.scopes.close(context.Background()) })
	globalInstructionsPath := cfg.GlobalInstructionsPath
	if globalInstructionsPath == "" {
		configDir, pathErr := daemonstate.DefaultConfigDir()
		if pathErr != nil {
			return fail(fmt.Errorf("resolve global instructions path: %w", pathErr))
		}
		globalInstructionsPath = filepath.Join(configDir, "AGENTS.md")
	}
	a.server = f.newServer(server.Config{
		RootContext: root, Store: a.store.store, ConsoleDevURL: cfg.ConsoleDevURL,
		Logger: a.logger, Logs: a.logs, Scopes: a.scopes.manager, Permissions: cfg.Permissions,
		AgentPermissions: cfg.AgentPermissions, PermissionTimeout: cfg.PermissionTimeout,
		Password: cfg.Password, Username: cfg.Username, InstanceID: cfg.InstanceID, Version: cfg.Version,
		GlobalInstructionsPath: globalInstructionsPath,
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
	if a.scopes.close != nil {
		errs = append(errs, a.scopes.close(ctx))
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
		newScopes: func(cfg execution.Config) (scopeResource, error) {
			manager, err := execution.NewManager(cfg)
			if err != nil {
				return scopeResource{}, err
			}
			return scopeResource{manager: manager, close: manager.CloseContext}, nil
		},
		newServer: func(cfg server.Config) lifecycleServer { return server.New(cfg) },
	}
}
