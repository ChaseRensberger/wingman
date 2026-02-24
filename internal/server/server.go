package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/chaserensberger/wingman/internal/storage"
)

type Server struct {
	store  storage.Store
	router *chi.Mux
}

type Config struct {
	Store storage.Store
}

func New(cfg Config) *Server {
	s := &Server{
		store:  cfg.Store,
		router: chi.NewRouter(),
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

func (s *Server) setupMiddleware() {
	s.router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(timeoutWithBypass(60*time.Second, shouldBypassTimeout))
	s.router.Use(jsonContentType)
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
	if strings.HasPrefix(path, "/formations/") && (strings.HasSuffix(path, "/run") || strings.HasSuffix(path, "/run/stream")) {
		return true
	}

	if strings.HasPrefix(path, "/sessions/") && strings.HasSuffix(path, "/message/stream") {
		return true
	}

	if strings.HasPrefix(path, "/fleets/") && strings.HasSuffix(path, "/run/stream") {
		return true
	}

	return false
}

func jsonContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) setupRoutes() {
	s.router.Get("/health", s.handleHealth)

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

	s.router.Route("/sessions", func(r chi.Router) {
		r.Post("/", s.handleCreateSession)
		r.Get("/", s.handleListSessions)
		r.Get("/{id}", s.handleGetSession)
		r.Put("/{id}", s.handleUpdateSession)
		r.Delete("/{id}", s.handleDeleteSession)
		r.Post("/{id}/message", s.handleMessageSession)
		r.Post("/{id}/message/stream", s.handleMessageStreamSession)
	})

	s.router.Route("/fleets", func(r chi.Router) {
		r.Post("/", s.handleCreateFleet)
		r.Get("/", s.handleListFleets)
		r.Get("/{id}", s.handleGetFleet)
		r.Put("/{id}", s.handleUpdateFleet)
		r.Delete("/{id}", s.handleDeleteFleet)
		r.Post("/{id}/run", s.handleRunFleet)
		r.Post("/{id}/run/stream", s.handleRunFleetStream)
	})

	s.router.Route("/formations", func(r chi.Router) {
		r.Post("/", s.handleCreateFormation)
		r.Get("/", s.handleListFormations)
		r.Get("/{id}", s.handleGetFormation)
		r.Get("/{id}/report", s.handleGetFormationReport)
		r.Put("/{id}", s.handleUpdateFormation)
		r.Delete("/{id}", s.handleDeleteFormation)
		r.Get("/{id}/export", s.handleExportFormation)
		r.Post("/{id}/run", s.handleRunFormation)
		r.Post("/{id}/run/stream", s.handleRunFormationStream)
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) ListenAndServe(addr string) error {
	log.Printf("Starting server on %s", addr)
	return http.ListenAndServe(addr, s.router)
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
