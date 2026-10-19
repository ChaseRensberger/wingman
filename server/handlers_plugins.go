package server

import (
	"net/http"

	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/api"
)

// PluginRegistration defines a bundled plugin and its server-wide activation state.
type PluginRegistration struct {
	ID      string
	Enabled bool
	Factory func() plugin.Plugin
}

func (s *Server) runtimePlugins() []plugin.Plugin {
	plugins := make([]plugin.Plugin, 0, len(s.plugins))
	for _, registration := range s.plugins {
		if registration.Enabled {
			plugins = append(plugins, registration.Factory())
		}
	}
	return plugins
}

func (s *Server) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	response := api.PluginsResponse{Plugins: make([]api.RegisteredPlugin, 0, len(s.plugins))}
	for _, registration := range s.plugins {
		response.Plugins = append(response.Plugins, api.RegisteredPlugin{ID: registration.ID, Enabled: registration.Enabled})
	}
	writeJSON(w, http.StatusOK, response)
}

type rpcPluginsResponse struct {
	Plugins any `json:"plugins"`
	Errors  any `json:"errors,omitempty"`
}

func (s *Server) handleListRPCPlugins(w http.ResponseWriter, r *http.Request) {
	scope, release, err := s.executionScope(r.Context(), "")
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer release()
	if scope == nil || scope.Plugins() == nil {
		writeJSON(w, http.StatusOK, rpcPluginsResponse{Plugins: []any{}})
		return
	}
	plugins, errs := scope.Plugins().Status()
	writeJSON(w, http.StatusOK, rpcPluginsResponse{Plugins: plugins, Errors: errs})
}

func (s *Server) handleReloadRPCPlugins(w http.ResponseWriter, r *http.Request) {
	scope, release, err := s.executionScope(r.Context(), "")
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer release()
	if scope == nil || scope.Plugins() == nil {
		writeJSON(w, http.StatusOK, rpcPluginsResponse{Plugins: []any{}})
		return
	}
	if err := scope.Plugins().Reload(r.Context()); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	plugins, errs := scope.Plugins().Status()
	writeJSON(w, http.StatusOK, rpcPluginsResponse{Plugins: plugins, Errors: errs})
}
