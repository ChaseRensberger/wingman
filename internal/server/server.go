package server

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

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
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Timeout(60 * time.Second))
	s.router.Use(jsonContentType)
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

	// s.router.Route("/fleets", func(r chi.Router) {
	// 	r.Post("/", s.handleCreateFleet)
	// 	r.Get("/", s.handleListFleets)
	// 	r.Get("/{id}", s.handleGetFleet)
	// 	r.Put("/{id}", s.handleUpdateFleet)
	// 	r.Delete("/{id}", s.handleDeleteFleet)
	// 	r.Post("/{id}/start", s.handleStartFleet)
	// 	r.Post("/{id}/stop", s.handleStopFleet)
	// 	r.Post("/{id}/submit", s.handleSubmitFleet)
	// })
	//
	// s.router.Route("/formations", func(r chi.Router) {
	// 	r.Post("/", s.handleCreateFormation)
	// 	r.Get("/", s.handleListFormations)
	// 	r.Get("/{id}", s.handleGetFormation)
	// 	r.Put("/{id}", s.handleUpdateFormation)
	// 	r.Delete("/{id}", s.handleDeleteFormation)
	// 	r.Post("/{id}/start", s.handleStartFormation)
	// 	r.Post("/{id}/stop", s.handleStopFormation)
	// 	r.Post("/{id}/message", s.handleMessageFormation)
	// })
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
