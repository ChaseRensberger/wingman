package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type mcpResponse struct {
	Servers any `json:"servers"`
}

func (s *Server) handleListMCP(w http.ResponseWriter, r *http.Request) {
	scope, release, err := s.executionScope(r.Context(), "")
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer release()
	if scope == nil || scope.MCP() == nil {
		writeJSON(w, http.StatusOK, mcpResponse{Servers: []any{}})
		return
	}
	writeJSON(w, http.StatusOK, mcpResponse{Servers: scope.MCP().Status()})
}

func (s *Server) handleConnectMCP(w http.ResponseWriter, r *http.Request) {
	scope, release, err := s.executionScope(r.Context(), "")
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer release()
	if scope == nil || scope.MCP() == nil {
		s.writeError(w, http.StatusNotFound, "MCP is not configured")
		return
	}
	name := chi.URLParam(r, "name")
	if err := scope.MCP().Connect(r.Context(), name); err != nil {
		s.writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, mcpResponse{Servers: scope.MCP().Status()})
}

func (s *Server) handleDisconnectMCP(w http.ResponseWriter, r *http.Request) {
	scope, release, err := s.executionScope(r.Context(), "")
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer release()
	if scope == nil || scope.MCP() == nil {
		s.writeError(w, http.StatusNotFound, "MCP is not configured")
		return
	}
	name := chi.URLParam(r, "name")
	if err := scope.MCP().Disconnect(name); err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, mcpResponse{Servers: scope.MCP().Status()})
}

func (s *Server) handleAuthMCP(w http.ResponseWriter, r *http.Request) {
	s.writeError(w, http.StatusNotImplemented, "MCP OAuth is not implemented yet")
}

func (s *Server) handleLogoutMCP(w http.ResponseWriter, r *http.Request) {
	s.writeError(w, http.StatusNotImplemented, "MCP OAuth is not implemented yet")
}
