package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/execution"
	"github.com/chaserensberger/wingman/internal/observability"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/store"
	consoleui "github.com/chaserensberger/wingman/web/apps/console"
)

type Server struct {
	store              store.Store
	router             *chi.Mux
	runs               *sessionRunManager
	permissionRequests *permissionRequestManager
	events             *sessionEventBroker
	webDevURL          string
	logger             *slog.Logger
	logs               *observability.LogBuffer
	scopes             *execution.Manager
	providers          *provider.Registry
	permissions        permission.Ruleset
	agentPermissions   map[string]permission.Ruleset
	oauth              *oauthManager

	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	startMu   sync.Mutex
	started   bool
	startDone chan struct{}
	startErr  error
	closeOnce sync.Once

	inflightMu     sync.Mutex
	inflightClosed bool
	inflight       sync.WaitGroup
}

type Config struct {
	// RootContext owns the server's background work. A nil context uses
	// context.Background.
	RootContext      context.Context
	Store            store.Store
	WebDevURL        string
	Logger           *slog.Logger
	Logs             *observability.LogBuffer
	Scopes           *execution.Manager
	Permissions      permission.Ruleset
	AgentPermissions map[string]permission.Ruleset
	// PermissionTimeout bounds interactive permission requests. Values less
	// than or equal to zero use the five-minute default.
	PermissionTimeout time.Duration
}

func New(cfg Config) *Server {
	root := cfg.RootContext
	rootProvided := root != nil
	if root == nil {
		root = context.Background()
	}
	ctx, cancel := context.WithCancel(root)
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	var providers *provider.Registry
	if cfg.Scopes != nil {
		providers = cfg.Scopes.Providers()
	}
	if providers == nil {
		providers, _ = provider.NewRegistry(nil)
	}
	s := &Server{
		store:            cfg.Store,
		router:           chi.NewRouter(),
		events:           newSessionEventBroker(),
		webDevURL:        cfg.WebDevURL,
		logger:           logger,
		logs:             cfg.Logs,
		scopes:           cfg.Scopes,
		providers:        providers,
		permissions:      cfg.Permissions,
		agentPermissions: cfg.AgentPermissions,
		oauth:            newOAuthManager(ctx, cfg.Store),
		shutdownCtx:      ctx,
		shutdownCancel:   cancel,
	}
	s.runs = newSessionRunManager(s)
	s.permissionRequests = newPermissionRequestManager(s, cfg.PermissionTimeout)

	s.setupMiddleware()
	s.setupRoutes()
	if rootProvided {
		go func() {
			<-ctx.Done()
			_ = s.Close(context.Background())
		}()
	}

	return s
}

func (s *Server) setupMiddleware() {
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(s.requestLogger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(timeoutWithBypass(60*time.Second, shouldBypassTimeout))
	s.router.Use(jsonContentType)
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		duration := time.Since(start)
		attrs := []any{
			"request_id", middleware.GetReqID(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"route", chi.RouteContext(r.Context()).RoutePattern(),
			"status", status,
			"bytes", ww.BytesWritten(),
			"duration_ms", duration.Milliseconds(),
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
		}
		if q := r.URL.RawQuery; q != "" {
			attrs = append(attrs, "query", q)
		}
		if clientID := r.Header.Get("X-Wingman-Client"); clientID != "" {
			attrs = append(attrs, "client_id", clientID)
		}

		switch {
		case status >= 500:
			s.logger.Error("http request", attrs...)
		case status >= 400:
			s.logger.Warn("http request", attrs...)
		default:
			s.logger.Info("http request", attrs...)
		}
	})
}

func timeoutWithBypass(timeout time.Duration, bypass func(*http.Request) bool) func(http.Handler) http.Handler {
	timed := middleware.Timeout(timeout)
	return func(next http.Handler) http.Handler {
		timedNext := timed(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if bypass != nil && bypass(r) {
				next.ServeHTTP(w, r)
				return
			}
			timedNext.ServeHTTP(w, r)
		})
	}
}

func shouldBypassTimeout(r *http.Request) bool {
	path := r.URL.Path
	if strings.HasPrefix(path, "/sessions/") && strings.Contains(path, "/events") {
		return true
	}
	if path == "/run" {
		return true
	}

	return false
}

func jsonContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" ||
			path == "/catalog" ||
			strings.HasPrefix(path, "/health") ||
			strings.HasPrefix(path, "/provider") ||
			strings.HasPrefix(path, "/agents") ||
			strings.HasPrefix(path, "/clients") ||
			strings.HasPrefix(path, "/logs") ||
			strings.HasPrefix(path, "/mcp") ||
			strings.HasPrefix(path, "/plugins") ||
			strings.HasPrefix(path, "/tools") ||
			strings.HasPrefix(path, "/workspaces") ||
			strings.HasPrefix(path, "/sessions") ||
			strings.HasPrefix(path, "/run") {
			w.Header().Set("Content-Type", "application/json")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) setupRoutes() {
	s.router.Get("/", s.handleRoot)
	s.router.Get("/health", s.handleHealth)
	s.router.Get("/logs", s.handleLogs)
	s.router.Route("/plugins", func(r chi.Router) {
		r.Get("/", s.handleListPlugins)
		r.Post("/reload", s.handleReloadPlugins)
	})
	s.router.Route("/mcp", func(r chi.Router) {
		r.Get("/", s.handleListMCP)
		r.Post("/{name}/connect", s.handleConnectMCP)
		r.Post("/{name}/disconnect", s.handleDisconnectMCP)
		r.Post("/{name}/auth", s.handleAuthMCP)
		r.Delete("/{name}/auth", s.handleLogoutMCP)
	})
	s.router.Get("/tools", s.handleListTools)
	s.router.Get("/catalog", s.handleCatalog)
	s.router.Get("/catalog/labs/{id}/logo", s.handleCatalogLabLogo)

	s.router.Route("/provider", func(r chi.Router) {
		r.Get("/", s.handleListProviders)
		r.Get("/auth", s.handleGetProvidersAuth)
		r.Put("/auth", s.handleSetProvidersAuth)
		r.Delete("/auth/{provider}", s.handleDeleteProviderAuth)
		r.Post("/{name}/oauth/authorize", s.handleProviderOAuthAuthorize)
		r.Get("/{name}/oauth/{attempt}", s.handleProviderOAuthStatus)
		r.Delete("/{name}/oauth/{attempt}", s.handleProviderOAuthCancel)
		r.Get("/{name}", s.handleGetProvider)
		r.Get("/{name}/models", s.handleListProviderModels)
		r.Get("/{name}/models/{model}", s.handleGetProviderModel)
	})

	s.router.Route("/agents", func(r chi.Router) {
		r.Get("/", s.handleListAgents)
		r.Post("/", s.handleCreateAgent)
		r.Get("/{id}", s.handleGetAgent)
		r.Put("/{id}", s.handleUpdateAgent)
		r.Delete("/{id}", s.handleDeleteAgent)
	})

	s.router.Route("/clients", func(r chi.Router) {
		r.Get("/", s.handleListClients)
		r.Post("/", s.handleCreateClient)
		r.Get("/{id}", s.handleGetClient)
	})

	s.router.Route("/workspaces", func(r chi.Router) {
		r.Get("/", s.handleListWorkspaces)
		r.Post("/", s.handleCreateWorkspace)
		r.Get("/{id}", s.handleGetWorkspace)
		r.Put("/{id}", s.handleUpdateWorkspace)
		r.Delete("/{id}", s.handleDeleteWorkspace)
		r.Get("/{id}/sessions", s.handleListWorkspaceSessions)
	})
	s.router.Get("/filesystem/directories", s.handleListDirectories)

	s.router.Route("/sessions", func(r chi.Router) {
		r.Post("/", s.handleCreateSession)
		r.Get("/", s.handleListSessions)
		r.Get("/{id}", s.handleGetSession)
		r.Get("/{id}/model-calls", s.handleListSessionModelCalls)
		r.Get("/{id}/tool-uses", s.handleListSessionToolUses)
		r.Get("/{id}/permission-requests", s.handleListPermissionRequests)
		r.Get("/{id}/permission-grants", s.handleListPermissionGrants)
		r.Post("/{id}/permission-requests/{requestID}/reply", s.handleReplyPermissionRequest)
		r.Post("/{id}/rename", s.handleRenameSession)
		r.Post("/{id}/move", s.handleMoveSession)
		r.Delete("/{id}", s.handleDeleteSession)
		r.Get("/{id}/events", s.handleSessionEvents)
		r.Get("/{id}/events/history", s.handleSessionEventsHistory)
		r.Post("/{id}/message", s.handleMessageSession)
		r.Post("/{id}/abort", s.handleAbortSession)
		r.Get("/{id}/runs", s.handleListSessionRuns)
		r.Get("/{id}/runs/{runID}", s.handleGetSessionRun)
		r.Post("/{id}/runs/{runID}/abort", s.handleAbortSessionRun)
	})

	s.router.Post("/run", s.handleRun)

	s.mountWebUI()
}

func (s *Server) mountWebUI() {
	if s.webDevURL != "" {
		devURL, err := url.Parse(s.webDevURL)
		if err != nil {
			s.logger.Error("invalid web dev url", "url", s.webDevURL, "error", err)
			return
		}
		proxy := httputil.NewSingleHostReverseProxy(devURL)
		s.router.Handle("/console", proxy)
		s.router.Handle("/console/*", proxy)
		return
	}

	dist, err := fs.Sub(consoleui.Dist, consoleui.DistRoot)
	if err != nil {
		s.logger.Error("web UI unavailable", "error", err)
		return
	}
	files := http.FileServer(http.FS(dist))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		servePath := strings.TrimPrefix(r.URL.Path, "/console")
		if servePath == "" || servePath == "/" {
			servePath = "/"
		}
		name := strings.TrimPrefix(path.Clean(servePath), "/")
		if stat, err := fs.Stat(dist, name); err != nil || stat.IsDir() {
			servePath = "/"
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = servePath
		files.ServeHTTP(w, r2)
	})
	s.router.Handle("/console", handler)
	s.router.Handle("/console/*", handler)
}

// Ephemeral reports whether the server was created with a nil store,
// meaning no persistence and CRUD endpoints return 501.
func (s *Server) Ephemeral() bool { return s.store == nil }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// Start recovers durable state and starts local queue reconciliation. It does
// not own an HTTP listener; callers serve the Server directly as an
// http.Handler.
func (s *Server) Start(ctx context.Context) error {
	s.startMu.Lock()
	if s.started {
		done, err := s.startDone, s.startErr
		s.startMu.Unlock()
		if done == nil {
			return err
		}
		select {
		case <-done:
			s.startMu.Lock()
			err = s.startErr
			s.startMu.Unlock()
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if s.shutdownCtx.Err() != nil {
		s.startMu.Unlock()
		return context.Canceled
	}
	s.started = true
	s.startDone = make(chan struct{})
	done := s.startDone
	s.startMu.Unlock()

	var err error
	if s.store != nil {
		err = s.recoverStartup(ctx)
	}

	s.startMu.Lock()
	s.startErr = err
	close(done)
	s.startMu.Unlock()
	return err
}

func (s *Server) recoverStartup(ctx context.Context) error {
	transitions, err := s.store.InterruptPendingPermissionRequests(ctx)
	if err != nil {
		return fmt.Errorf("interrupt pending permission requests: %w", err)
	}
	for _, transition := range transitions {
		if transition.Changed {
			s.events.publish(transition.Event)
		}
	}
	if err := s.store.InterruptActiveToolUses(ctx); err != nil {
		return fmt.Errorf("interrupt active tool uses: %w", err)
	}
	runs, err := s.store.ListRunningSessionRuns(ctx)
	if err != nil {
		return fmt.Errorf("list running session runs: %w", err)
	}
	for _, run := range runs {
		if err := s.store.InterruptActiveModelCalls(ctx, run.ID, "process_interrupted", "process interrupted during run"); err != nil {
			return fmt.Errorf("interrupt active model calls for run %s: %w", run.ID, err)
		}
		if err := session.RecoverRunMessages(ctx, s.store, run.SessionID, run.ID); err != nil {
			return fmt.Errorf("recover messages for run %s: %w", run.ID, err)
		}
		transition, err := s.store.SettleSessionRun(ctx, store.SessionRunSettlement{ID: run.ID, ExpectedStatus: store.SessionRunStatusRunning, Status: store.SessionRunStatusAborted, ErrorType: "process_interrupted", ErrorMessage: "process interrupted during run", EventData: map[string]any{"error_type": "process_interrupted", "error_message": "process interrupted during run"}})
		if err != nil {
			return fmt.Errorf("abort running session run %s: %w", run.ID, err)
		}
		if transition.Changed {
			s.events.publish(transition.Event)
		}
	}
	if err := s.runs.resumeQueued(ctx); err != nil {
		return fmt.Errorf("resume queued session runs: %w", err)
	}
	s.runs.startReconciler()
	return nil
}

// Close cancels server-owned work and waits for it to finish. A timed-out
// close may be retried with a new context.
func (s *Server) Close(ctx context.Context) error {
	s.closeOnce.Do(func() {
		s.shutdownCancel()
		if s.runs != nil {
			s.runs.stop()
		}
		s.inflightMu.Lock()
		s.inflightClosed = true
		s.inflightMu.Unlock()
	})

	var errs []error
	if s.runs != nil {
		errs = append(errs, s.runs.wait(ctx))
	}
	if s.oauth != nil {
		errs = append(errs, s.oauth.Close(ctx))
	}
	errs = append(errs, s.waitInflight(ctx))
	return errors.Join(errs...)
}

func (s *Server) trackInflight() func() {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	if s.inflightClosed {
		return func() {}
	}
	s.inflight.Add(1)
	return s.inflight.Done
}

func (s *Server) waitInflight(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ShutdownCtx is cancelled when the server begins closing.
func (s *Server) ShutdownCtx() context.Context { return s.shutdownCtx }

func (s *Server) executionScope(ctx context.Context, workDir string) (*execution.Scope, func(), error) {
	if s.scopes == nil {
		return nil, func() {}, nil
	}
	lease, err := s.scopes.Acquire(ctx, workDir)
	if err != nil {
		return nil, nil, err
	}
	return lease.Scope(), func() { _ = lease.Close(context.Background()) }, nil
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

func (s *Server) ephemeralNotImplemented(w http.ResponseWriter) {
	writeError(w, http.StatusNotImplemented, "persistence is disabled; this server is running in ephemeral mode")
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Health string `json:"health"`
		Web    string `json:"web"`
	}{
		Name:   "wingman",
		Status: "ok",
		Health: "/health",
		Web:    "/console",
	})
}
