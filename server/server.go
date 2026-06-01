package server

import (
	"context"
	"encoding/json"
	"errors"
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

	"github.com/chaserensberger/wingman/internal/observability"
	"github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/pluginhost"
	"github.com/chaserensberger/wingman/store"
	webui "github.com/chaserensberger/wingman/web"
)

type Server struct {
	store     store.Store
	router    *chi.Mux
	aborts    *abortRegistry
	webDevURL string
	logger    *slog.Logger
	logs      *observability.LogBuffer
	plugins   *pluginhost.Manager
	providers map[string]provider.ProviderConfig

	// shutdownCtx is cancelled when Shutdown is called. SSE handlers
	// (and any other long-lived in-flight request) should select on its
	// Done channel so they can return promptly during a drain instead
	// of blocking http.Server.Shutdown.
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	// inflight tracks SSE handlers explicitly. http.Server.Shutdown
	// already waits for unary handlers to return (they'll all be done
	// in <60s thanks to the timeout middleware), but streaming handlers
	// can run for minutes. We use this to wait on them after cancelling
	// shutdownCtx.
	inflight sync.WaitGroup
}

type Config struct {
	Store     store.Store
	WebDevURL string
	Logger    *slog.Logger
	Logs      *observability.LogBuffer
	Plugins   *pluginhost.Manager
	Providers map[string]provider.ProviderConfig
}

func New(cfg Config) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	provider.RegisterConfig(cfg.Providers)
	s := &Server{
		store:          cfg.Store,
		router:         chi.NewRouter(),
		aborts:         newAbortRegistry(),
		webDevURL:      cfg.WebDevURL,
		logger:         logger,
		logs:           cfg.Logs,
		plugins:        cfg.Plugins,
		providers:      cfg.Providers,
		shutdownCtx:    ctx,
		shutdownCancel: cancel,
	}

	s.setupMiddleware()
	s.setupRoutes()

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
	if strings.HasPrefix(path, "/sessions/") && strings.HasSuffix(path, "/message/stream") {
		return true
	}

	return false
}

func jsonContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" ||
			strings.HasPrefix(path, "/health") ||
			strings.HasPrefix(path, "/provider") ||
			strings.HasPrefix(path, "/agents") ||
			strings.HasPrefix(path, "/clients") ||
			strings.HasPrefix(path, "/logs") ||
			strings.HasPrefix(path, "/plugins") ||
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

	s.router.Route("/provider", func(r chi.Router) {
		r.Get("/", s.handleListProviders)
		r.Get("/auth", s.handleGetProvidersAuth)
		r.Put("/auth", s.handleSetProvidersAuth)
		r.Delete("/auth/{provider}", s.handleDeleteProviderAuth)
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

	s.router.Route("/sessions", func(r chi.Router) {
		r.Post("/", s.handleCreateSession)
		r.Get("/", s.handleListSessions)
		r.Get("/{id}", s.handleGetSession)
		r.Put("/{id}", s.handleUpdateSession)
		r.Delete("/{id}", s.handleDeleteSession)
		r.Post("/{id}/message", s.handleMessageSession)
		r.Post("/{id}/message/stream", s.handleMessageStreamSession)
		r.Post("/{id}/abort", s.handleAbortSession)
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
		s.router.Handle("/web", proxy)
		s.router.Handle("/web/*", proxy)
		return
	}

	dist, err := fs.Sub(webui.Dist, webui.DistRoot)
	if err != nil {
		s.logger.Error("web UI unavailable", "error", err)
		return
	}
	files := http.FileServer(http.FS(dist))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		servePath := strings.TrimPrefix(r.URL.Path, "/web")
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
	s.router.Handle("/web", handler)
	s.router.Handle("/web/*", handler)
}

// Ephemeral reports whether the server was created with a nil store,
// meaning no persistence and CRUD endpoints return 501.
func (s *Server) Ephemeral() bool { return s.store == nil }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// ListenAndServe starts the HTTP server on addr and blocks until the
// server is shut down. Returns nil on a clean shutdown, or the listener
// error otherwise. Shutdown is initiated via Shutdown.
//
// Kept for backward compatibility. New callers should prefer Serve,
// which lets them own the listener / TLS config / etc.
func (s *Server) ListenAndServe(addr string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}
	return s.Serve(srv)
}

// Serve runs srv (caller-owned) until it terminates. The server's
// Handler is overwritten with our router. Returns nil on a graceful
// shutdown, the underlying error otherwise.
func (s *Server) Serve(srv *http.Server) error {
	srv.Handler = s.router
	s.logger.Info("server starting", "addr", srv.Addr)
	err := srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown initiates a graceful drain. It:
//  1. cancels the server's shutdownCtx so SSE / long-lived handlers
//     can return promptly;
//  2. calls srv.Shutdown(ctx), which stops accepting new connections
//     and waits for active handlers to return;
//  3. waits for any tracked streaming handlers to finish via the
//     inflight WaitGroup (with the same ctx as a deadline).
//
// Returns the first non-nil error encountered. Pass a deadlined ctx to
// bound shutdown time; passing context.Background means "wait forever".
//
// It is safe to call Shutdown multiple times; the second call is a
// no-op for the cancellation step but will still call srv.Shutdown
// (which is itself idempotent).
func (s *Server) Shutdown(ctx context.Context, srv *http.Server) error {
	s.shutdownCancel()

	var firstErr error
	if err := srv.Shutdown(ctx); err != nil {
		firstErr = err
	}

	// Wait for streaming handlers, but don't exceed ctx's deadline.
	done := make(chan struct{})
	go func() {
		s.inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		if firstErr == nil {
			firstErr = ctx.Err()
		}
	}
	return firstErr
}

// trackInflight is called by streaming handlers (SSE) to register
// themselves with the WaitGroup so Shutdown can wait for them. The
// returned func MUST be deferred. ShutdownCtx returns the server's
// drain-signal context for the same handlers to monitor.
func (s *Server) trackInflight() func() {
	s.inflight.Add(1)
	return s.inflight.Done
}

// ShutdownCtx returns a context that is cancelled when Shutdown is
// called. Streaming handlers select on its Done channel to abort
// promptly during a drain.
func (s *Server) ShutdownCtx() context.Context { return s.shutdownCtx }

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
		Web:    "/web",
	})
}
