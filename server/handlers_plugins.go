package server

import "net/http"

type pluginsResponse struct {
	Plugins any `json:"plugins"`
	Errors  any `json:"errors,omitempty"`
}

func (s *Server) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	scope, release, err := s.executionScope(r.Context(), "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer release()
	if scope == nil || scope.Plugins() == nil {
		writeJSON(w, http.StatusOK, pluginsResponse{Plugins: []any{}})
		return
	}
	plugins, errs := scope.Plugins().Status()
	writeJSON(w, http.StatusOK, pluginsResponse{Plugins: plugins, Errors: errs})
}

func (s *Server) handleReloadPlugins(w http.ResponseWriter, r *http.Request) {
	scope, release, err := s.executionScope(r.Context(), "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer release()
	if scope == nil || scope.Plugins() == nil {
		writeJSON(w, http.StatusOK, pluginsResponse{Plugins: []any{}})
		return
	}
	if err := scope.Plugins().Reload(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	plugins, errs := scope.Plugins().Status()
	writeJSON(w, http.StatusOK, pluginsResponse{Plugins: plugins, Errors: errs})
}
