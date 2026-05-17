package server

import "net/http"

type pluginsResponse struct {
	Plugins any `json:"plugins"`
	Errors  any `json:"errors,omitempty"`
}

func (s *Server) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	if s.plugins == nil {
		writeJSON(w, http.StatusOK, pluginsResponse{Plugins: []any{}})
		return
	}
	plugins, errs := s.plugins.Status()
	writeJSON(w, http.StatusOK, pluginsResponse{Plugins: plugins, Errors: errs})
}

func (s *Server) handleReloadPlugins(w http.ResponseWriter, r *http.Request) {
	if s.plugins == nil {
		writeJSON(w, http.StatusOK, pluginsResponse{Plugins: []any{}})
		return
	}
	if err := s.plugins.Reload(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	plugins, errs := s.plugins.Status()
	writeJSON(w, http.StatusOK, pluginsResponse{Plugins: plugins, Errors: errs})
}
