package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/api"
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
	protocol           huma.API
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
	credential         string
	instanceID         string
	version            string
	consoleCookie      bool
	ready              atomic.Bool

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
	Credential        string
	InstanceID        string
	Version           string
	ConsoleCookie     bool
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
		credential:       cfg.Credential,
		instanceID:       cfg.InstanceID,
		version:          cfg.Version,
		consoleCookie:    cfg.ConsoleCookie,
		shutdownCtx:      ctx,
		shutdownCancel:   cancel,
	}
	s.runs = newSessionRunManager(s)
	s.permissionRequests = newPermissionRequestManager(s, cfg.PermissionTimeout)

	s.setupMiddleware()
	s.setupOpenAPI()
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
	s.router.Use(requestIDHeader)
	s.router.Use(middleware.RealIP)
	s.router.Use(s.requestLogger)
	s.router.Use(s.recoverer)
	s.router.Use(s.timeoutWithBypass(60*time.Second, shouldBypassTimeout))
	s.router.Use(s.authenticate)
}

const consoleSessionCookie = "wingman_session"

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.credential == "" || r.URL.Path == "/health" || r.URL.Path == "/console" || strings.HasPrefix(r.URL.Path, "/console/") {
			next.ServeHTTP(w, r)
			return
		}
		credential := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if credential == r.Header.Get("Authorization") {
			if cookie, err := r.Cookie(consoleSessionCookie); err == nil {
				credential = cookie.Value
			}
		}
		if subtle.ConstantTimeCompare([]byte(credential), []byte(s.credential)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="wingman"`)
			s.writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) setConsoleSession(w http.ResponseWriter, r *http.Request) {
	if !s.consoleCookie || s.credential == "" || !requestHostIsLoopback(r.Host) {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: s.consoleSessionCookieName(), Value: s.credential, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) consoleSessionCookieName() string { return consoleSessionCookie }

func requestHostIsLoopback(host string) bool {
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	} else {
		host = strings.Trim(host, "[]")
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func requestIDHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", middleware.GetReqID(r.Context()))
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}
				s.logger.Error("http handler panic", "request_id", middleware.GetReqID(r.Context()), "panic", recovered, "stack", string(debug.Stack()))
				s.writeError(w, http.StatusInternalServerError, fmt.Sprint(recovered))
			}
		}()
		next.ServeHTTP(w, r)
	})
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

func (s *Server) timeoutWithBypass(timeout time.Duration, bypass func(*http.Request) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if bypass != nil && bypass(r) {
				next.ServeHTTP(w, r)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer func() {
				cancel()
				if errors.Is(ctx.Err(), context.DeadlineExceeded) && !responseCommitted(w) {
					s.writeError(w, http.StatusGatewayTimeout, ctx.Err().Error())
				}
			}()
			next.ServeHTTP(w, r.WithContext(ctx))
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

func (s *Server) setupRoutes() {
	s.registerJSON(http.MethodGet, "/", "getService", "Describe the Wingman service", nil, http.StatusOK, rootResponse{}, s.handleRoot)
	s.registerJSON(http.MethodGet, "/health", "getHealth", "Check daemon health", nil, http.StatusOK, api.StatusResponse{}, s.handleHealth)
	s.registerJSONStatuses(http.MethodGet, "/ready", "getReadiness", "Check daemon readiness", nil, map[int]any{http.StatusOK: api.ReadinessResponse{}, http.StatusServiceUnavailable: api.ReadinessResponse{}}, s.handleReadiness)
	s.registerJSON(http.MethodGet, "/logs", "listLogs", "List recent daemon logs", nil, http.StatusOK, []observability.LogEntry{}, s.handleLogs)
	s.registerJSON(http.MethodGet, "/diagnostics", "getDiagnostics", "Get bounded daemon operational diagnostics", nil, http.StatusOK, api.DiagnosticsResponse{}, s.handleDiagnostics)
	s.registerJSON(http.MethodGet, "/plugins", "listPlugins", "List plugin status", nil, http.StatusOK, pluginsResponse{}, s.handleListPlugins)
	s.registerJSON(http.MethodPost, "/plugins/reload", "reloadPlugins", "Reload plugins", nil, http.StatusOK, pluginsResponse{}, s.handleReloadPlugins)
	s.registerJSON(http.MethodGet, "/mcp", "listMCPServers", "List MCP server status", nil, http.StatusOK, mcpResponse{}, s.handleListMCP)
	s.registerJSON(http.MethodPost, "/mcp/{name}/connect", "connectMCPServer", "Connect an MCP server", nil, http.StatusOK, mcpResponse{}, s.handleConnectMCP)
	s.registerJSON(http.MethodPost, "/mcp/{name}/disconnect", "disconnectMCPServer", "Disconnect an MCP server", nil, http.StatusOK, mcpResponse{}, s.handleDisconnectMCP)
	s.registerErrorOnly(http.MethodPost, "/mcp/{name}/auth", "authorizeMCPServer", "Authorize an MCP server", s.handleAuthMCP)
	s.registerErrorOnly(http.MethodDelete, "/mcp/{name}/auth", "logoutMCPServer", "Remove MCP authorization", s.handleLogoutMCP)
	s.registerJSON(http.MethodGet, "/tools", "listTools", "List available tools", nil, http.StatusOK, toolCatalogResponse{}, s.handleListTools)
	s.registerJSON(http.MethodGet, "/catalog", "getModelCatalog", "Get the model catalog", nil, http.StatusOK, CatalogDTO{}, s.handleCatalog)
	s.registerBinary(http.MethodGet, "/catalog/labs/{id}/logo", "getCatalogLabLogo", "Get a catalog lab logo", "image/svg+xml", s.handleCatalogLabLogo)

	s.registerJSON(http.MethodGet, "/provider", "listProviders", "List model providers", nil, http.StatusOK, []ProviderDTO{}, s.handleListProviders)
	s.registerJSON(http.MethodGet, "/provider/auth", "getProviderAuth", "Get provider credential status", nil, http.StatusOK, ProvidersAuthResponse{}, s.handleGetProvidersAuth)
	s.registerJSON(http.MethodPut, "/provider/auth", "setProviderAuth", "Set provider credentials", SetProvidersAuthRequest{}, http.StatusOK, api.StatusResponse{}, s.handleSetProvidersAuth)
	s.registerJSON(http.MethodDelete, "/provider/auth/{provider}", "deleteProviderAuth", "Delete provider credentials", nil, http.StatusOK, api.StatusResponse{}, s.handleDeleteProviderAuth)
	s.registerJSON(http.MethodPost, "/provider/{name}/oauth/authorize", "authorizeProviderOAuth", "Start provider OAuth", providerOAuthRequest{}, http.StatusAccepted, oauthAttemptDTO{}, s.handleProviderOAuthAuthorize)
	s.registerJSON(http.MethodGet, "/provider/{name}/oauth/{attempt}", "getProviderOAuthAttempt", "Get provider OAuth status", nil, http.StatusOK, oauthAttemptDTO{}, s.handleProviderOAuthStatus)
	s.registerJSON(http.MethodDelete, "/provider/{name}/oauth/{attempt}", "cancelProviderOAuthAttempt", "Cancel provider OAuth", nil, http.StatusOK, api.StatusResponse{}, s.handleProviderOAuthCancel)
	s.registerJSON(http.MethodGet, "/provider/{name}", "getProvider", "Get a model provider", nil, http.StatusOK, ProviderDTO{}, s.handleGetProvider)
	s.registerJSON(http.MethodGet, "/provider/{name}/models", "listProviderModels", "List provider models", nil, http.StatusOK, map[string]ModelDTO{}, s.handleListProviderModels)
	s.registerJSON(http.MethodGet, "/provider/{name}/models/{model}", "getProviderModel", "Get a provider model", nil, http.StatusOK, ModelDTO{}, s.handleGetProviderModel)

	s.registerJSON(http.MethodGet, "/agents", "listAgents", "List agents", nil, http.StatusOK, []api.Agent{}, s.handleListAgents)
	s.registerJSON(http.MethodPost, "/agents", "createAgent", "Create an agent", api.CreateAgentRequest{}, http.StatusCreated, api.Agent{}, s.handleCreateAgent)
	s.registerJSON(http.MethodGet, "/agents/{id}", "getAgent", "Get an agent", nil, http.StatusOK, api.Agent{}, s.handleGetAgent)
	s.registerJSON(http.MethodPut, "/agents/{id}", "updateAgent", "Update an agent", api.UpdateAgentRequest{}, http.StatusOK, api.Agent{}, s.handleUpdateAgent)
	s.registerJSON(http.MethodDelete, "/agents/{id}", "deleteAgent", "Delete an agent", nil, http.StatusOK, api.StatusResponse{}, s.handleDeleteAgent)

	s.registerJSON(http.MethodGet, "/clients", "listClients", "List API clients", nil, http.StatusOK, []api.Client{}, s.handleListClients)
	s.registerJSON(http.MethodPost, "/clients", "createClient", "Create an API client", api.CreateClientRequest{}, http.StatusCreated, api.Client{}, s.handleCreateClient)
	s.registerJSON(http.MethodGet, "/clients/{id}", "getClient", "Get an API client", nil, http.StatusOK, api.Client{}, s.handleGetClient)

	s.registerJSON(http.MethodGet, "/workspaces", "listWorkspaces", "List Workspaces", nil, http.StatusOK, []api.Workspace{}, s.handleListWorkspaces)
	s.registerJSON(http.MethodPost, "/workspaces", "createWorkspace", "Create a Workspace", api.CreateWorkspaceRequest{}, http.StatusCreated, api.Workspace{}, s.handleCreateWorkspace)
	s.registerJSON(http.MethodGet, "/workspaces/{id}", "getWorkspace", "Get a Workspace", nil, http.StatusOK, api.Workspace{}, s.handleGetWorkspace)
	s.registerJSON(http.MethodPut, "/workspaces/{id}", "updateWorkspace", "Update a Workspace", api.UpdateWorkspaceRequest{}, http.StatusOK, api.Workspace{}, s.handleUpdateWorkspace)
	s.registerJSON(http.MethodDelete, "/workspaces/{id}", "deleteWorkspace", "Delete a Workspace", nil, http.StatusOK, api.StatusResponse{}, s.handleDeleteWorkspace)
	s.registerJSON(http.MethodGet, "/workspaces/{id}/sessions", "listWorkspaceSessions", "List Workspace sessions", nil, http.StatusOK, []api.Session{}, s.handleListWorkspaceSessions)
	s.registerJSONWithParameters(http.MethodGet, "/filesystem/directories", "listDirectories", "List filesystem directories", nil, http.StatusOK, directoryListing{}, []*huma.Param{queryParameter("path", huma.TypeString, "Directory to list")}, s.handleListDirectories)

	s.registerJSON(http.MethodPost, "/sessions", "createSession", "Create a session", api.CreateSessionRequest{}, http.StatusCreated, api.Session{}, s.handleCreateSession)
	s.registerJSON(http.MethodGet, "/sessions", "listSessions", "List sessions", nil, http.StatusOK, []api.Session{}, s.handleListSessions)
	s.registerJSON(http.MethodGet, "/sessions/{id}", "getSession", "Get a session", nil, http.StatusOK, api.SessionDetail{}, s.handleGetSession)
	s.registerJSON(http.MethodGet, "/sessions/{id}/model-calls", "listSessionModelCalls", "List session model calls", nil, http.StatusOK, []api.ModelCall{}, s.handleListSessionModelCalls)
	s.registerJSON(http.MethodGet, "/sessions/{id}/tool-uses", "listSessionToolUses", "List session tool uses", nil, http.StatusOK, []api.ToolUse{}, s.handleListSessionToolUses)
	s.registerJSON(http.MethodGet, "/sessions/{id}/permission-requests", "listPermissionRequests", "List session permission requests", nil, http.StatusOK, []api.PermissionRequest{}, s.handleListPermissionRequests)
	s.registerJSON(http.MethodGet, "/sessions/{id}/permission-grants", "listPermissionGrants", "List session permission grants", nil, http.StatusOK, []api.PermissionGrant{}, s.handleListPermissionGrants)
	s.registerJSON(http.MethodPost, "/sessions/{id}/permission-requests/{requestID}/reply", "replyPermissionRequest", "Reply to a permission request", api.PermissionReplyRequest{}, http.StatusOK, api.PermissionRequest{}, s.handleReplyPermissionRequest)
	s.registerJSON(http.MethodPost, "/sessions/{id}/rename", "renameSession", "Rename a session", api.RenameSessionRequest{}, http.StatusOK, api.Session{}, s.handleRenameSession)
	s.registerJSON(http.MethodPost, "/sessions/{id}/move", "moveSession", "Move a session", api.MoveSessionRequest{}, http.StatusOK, api.Session{}, s.handleMoveSession)
	s.registerJSONWithParameters(http.MethodDelete, "/sessions/{id}", "deleteSession", "Delete a session", nil, http.StatusOK, api.StatusResponse{}, []*huma.Param{{Name: "expected_version", In: "query", Required: true, Schema: &huma.Schema{Type: huma.TypeInteger, Format: "int64"}}}, s.handleDeleteSession)
	s.registerSessionEvents()
	s.registerJSONWithParameters(http.MethodGet, "/sessions/{id}/events/history", "listSessionEvents", "List durable session events", nil, http.StatusOK, api.SessionEventPage{}, []*huma.Param{queryParameter("after", huma.TypeInteger, "Exclusive durable event cursor"), queryParameter("limit", huma.TypeInteger, "Maximum page size")}, s.handleSessionEventsHistory)
	s.registerJSON(http.MethodPost, "/sessions/{id}/message", "messageSession", "Admit a session message", api.MessageSessionRequest{}, http.StatusAccepted, api.MessageSessionResponse{}, s.handleMessageSession)
	s.registerJSON(http.MethodPost, "/sessions/{id}/abort", "abortSession", "Abort active session runs", nil, http.StatusOK, api.AbortSessionResponse{}, s.handleAbortSession)
	s.registerJSON(http.MethodGet, "/sessions/{id}/runs", "listSessionRuns", "List session runs", nil, http.StatusOK, []api.SessionRun{}, s.handleListSessionRuns)
	s.registerJSON(http.MethodGet, "/sessions/{id}/runs/{runID}", "getSessionRun", "Get a session run", nil, http.StatusOK, api.SessionRun{}, s.handleGetSessionRun)
	s.registerJSONStatuses(http.MethodPost, "/sessions/{id}/runs/{runID}/abort", "abortSessionRun", "Abort a session run", nil, map[int]any{http.StatusOK: api.SessionRun{}, http.StatusAccepted: api.SessionRun{}}, s.handleAbortSessionRun)

	s.registerRunStream()
	s.router.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		s.writeError(w, http.StatusNotFound, "route not found")
	})
	s.router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", strings.Join(s.allowedMethods(r.URL.Path), ", "))
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	})

	s.mountWebUI()
}

func (s *Server) allowedMethods(routePath string) []string {
	candidates := []string{
		http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions,
	}
	allowed := make([]string, 0, len(candidates))
	for _, method := range candidates {
		if s.router.Match(chi.NewRouteContext(), method, routePath) {
			allowed = append(allowed, method)
		}
	}
	return allowed
}

func (s *Server) mountWebUI() {
	if s.webDevURL != "" {
		devURL, err := url.Parse(s.webDevURL)
		if err != nil {
			s.logger.Error("invalid web dev url", "url", s.webDevURL, "error", err)
			return
		}
		proxy := httputil.NewSingleHostReverseProxy(devURL)
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.setConsoleSession(w, r)
			proxy.ServeHTTP(w, r)
		})
		s.router.Handle("/console", handler)
		s.router.Handle("/console/*", handler)
		return
	}

	dist, err := fs.Sub(consoleui.Dist, consoleui.DistRoot)
	if err != nil {
		s.logger.Error("web UI unavailable", "error", err)
		return
	}
	files := http.FileServer(http.FS(dist))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.setConsoleSession(w, r)
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
	s.ready.Store(err == nil)
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
		s.ready.Store(false)
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, msg string) {
	requestID := w.Header().Get("X-Request-ID")
	if responseCommitted(w) {
		s.logger.Error("cannot write HTTP error after response committed", "request_id", requestID, "status", status, "error", msg)
		return
	}
	if status >= http.StatusInternalServerError && status != http.StatusNotImplemented {
		s.logger.Error("http request failed", "request_id", requestID, "status", status, "error", msg)
		msg = publicServerErrorMessage(status)
	}
	writeJSON(w, status, api.ErrorResponse{Error: api.Error{
		Code: codeForStatus(status), Message: msg, RequestID: requestID,
	}})
}

func responseCommitted(w http.ResponseWriter) bool {
	type statusWriter interface{ Status() int }
	ww, ok := w.(statusWriter)
	return ok && ww.Status() != 0
}

func codeForStatus(status int) api.ErrorCode {
	switch status {
	case http.StatusBadRequest:
		return api.ErrorCodeInvalidRequest
	case http.StatusUnauthorized:
		return api.ErrorCodeUnauthorized
	case http.StatusForbidden:
		return api.ErrorCodeForbidden
	case http.StatusNotFound:
		return api.ErrorCodeNotFound
	case http.StatusMethodNotAllowed:
		return api.ErrorCodeMethodNotAllowed
	case http.StatusConflict:
		return api.ErrorCodeConflict
	case http.StatusRequestEntityTooLarge:
		return api.ErrorCodePayloadTooLarge
	case http.StatusUnsupportedMediaType:
		return api.ErrorCodeUnsupportedMedia
	case http.StatusUnprocessableEntity:
		return api.ErrorCodeValidationFailed
	case http.StatusTooManyRequests:
		return api.ErrorCodeRateLimited
	case http.StatusInternalServerError:
		return api.ErrorCodeInternal
	case http.StatusNotImplemented:
		return api.ErrorCodeNotImplemented
	case http.StatusBadGateway:
		return api.ErrorCodeUpstream
	case http.StatusServiceUnavailable:
		return api.ErrorCodeUnavailable
	case http.StatusGatewayTimeout:
		return api.ErrorCodeTimeout
	default:
		return api.ErrorCodeRequestFailed
	}
}

func publicServerErrorMessage(status int) string {
	switch status {
	case http.StatusBadGateway:
		return "upstream service error"
	case http.StatusServiceUnavailable:
		return "service unavailable"
	case http.StatusGatewayTimeout:
		return "request timed out"
	default:
		return "internal server error"
	}
}

func (s *Server) ephemeralNotImplemented(w http.ResponseWriter) {
	s.writeError(w, http.StatusNotImplemented, "persistence is disabled; this server is running in ephemeral mode")
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadiness(w http.ResponseWriter, _ *http.Request) {
	response := api.ReadinessResponse{Ready: s.ready.Load(), InstanceID: s.instanceID, Version: s.version}
	status := http.StatusOK
	if !response.Ready {
		status = http.StatusServiceUnavailable
		response.Diagnostic = &api.ReadinessDiagnostic{Subsystem: "startup", RecoveryAction: "start the daemon"}
		s.startMu.Lock()
		if s.startErr != nil {
			response.Diagnostic = &api.ReadinessDiagnostic{Subsystem: "startup_recovery", RecoveryAction: "inspect daemon logs, correct the reported failure, then restart the daemon"}
		}
		s.startMu.Unlock()
	}
	writeJSON(w, status, response)
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	response := api.DiagnosticsResponse{ActiveRuns: s.runs.activeCount()}
	if s.store != nil {
		queued, err := s.store.CountQueuedSessionRuns(r.Context())
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		response.QueuedRuns = queued
	}
	if s.scopes != nil {
		response.ActiveScopes = s.scopes.Count()
	}
	response.EventSubscribers, response.SubscriberOverflows = s.events.diagnostics()
	writeJSON(w, http.StatusOK, response)
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
