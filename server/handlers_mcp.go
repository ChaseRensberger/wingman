package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type mcpResponse struct {
	Servers any `json:"servers"`
}

func (s *Server) handleListMCP(w http.ResponseWriter, r *http.Request) {
	if s.mcp == nil {
		writeJSON(w, http.StatusOK, mcpResponse{Servers: []any{}})
		return
	}
	writeJSON(w, http.StatusOK, mcpResponse{Servers: s.mcp.Status()})
}

func (s *Server) handleConnectMCP(w http.ResponseWriter, r *http.Request) {
	if s.mcp == nil {
		writeError(w, http.StatusNotFound, "MCP is not configured")
		return
	}
	name := chi.URLParam(r, "name")
	if err := s.mcp.Connect(r.Context(), name); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, mcpResponse{Servers: s.mcp.Status()})
}

func (s *Server) handleDisconnectMCP(w http.ResponseWriter, r *http.Request) {
	if s.mcp == nil {
		writeError(w, http.StatusNotFound, "MCP is not configured")
		return
	}
	name := chi.URLParam(r, "name")
	if err := s.mcp.Disconnect(name); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, mcpResponse{Servers: s.mcp.Status()})
}

func (s *Server) handleAuthMCP(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "MCP OAuth is not implemented yet")
}

func (s *Server) handleLogoutMCP(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "MCP OAuth is not implemented yet")
}
