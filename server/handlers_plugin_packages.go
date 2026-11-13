package server

import (
	"encoding/json"
	"net/http"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/plugin/protocol"
	"github.com/chaserensberger/wingman/pluginhost"
	"github.com/chaserensberger/wingman/pluginhub"
)

type pluginPackageRequest struct {
	Operation string `json:"operation" enum:"install,update,remove"`
	Source    string `json:"source"`
}

type pluginPackageResponse struct {
	Installation *pluginhub.Installation `json:"installation,omitempty"`
	Status       string                  `json:"status"`
}
type pluginTrustRequest struct {
	Directory string `json:"directory"`
	Trusted   bool   `json:"trusted"`
}
type pluginCallRequest struct {
	Plugin           string          `json:"plugin"`
	Method           string          `json:"method"`
	Input            json.RawMessage `json:"input"`
	WorkingDirectory string          `json:"working_directory,omitempty"`
}

func (s *Server) handlePluginPackages(w http.ResponseWriter, r *http.Request) {
	if !s.requireRoot(w, r) {
		return
	}
	if s.pluginInstaller == nil {
		s.writeError(w, http.StatusNotImplemented, "plugin package management is unavailable")
		return
	}
	list, err := s.pluginInstaller.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleManagePluginPackage(w http.ResponseWriter, r *http.Request) {
	if !s.requireRoot(w, r) {
		return
	}
	if s.pluginInstaller == nil {
		s.writeError(w, http.StatusNotImplemented, "plugin package management is unavailable")
		return
	}
	var input pluginPackageRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid plugin package request")
		return
	}
	switch input.Operation {
	case "install", "update":
		installation, err := s.pluginInstaller.Install(r.Context(), input.Source, input.Operation == "update")
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, pluginPackageResponse{Status: "installed", Installation: &installation})
	case "remove":
		if err := s.pluginInstaller.Remove(r.Context(), input.Source); err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, pluginPackageResponse{Status: "removed"})
	default:
		s.writeError(w, http.StatusBadRequest, "operation must be install, update, or remove")
	}
}

func (s *Server) handlePluginCatalog(w http.ResponseWriter, r *http.Request) {
	if !s.requireRoot(w, r) {
		return
	}
	if s.pluginInstaller == nil {
		s.writeError(w, http.StatusNotImplemented, "plugin package management is unavailable")
		return
	}
	catalog, err := s.pluginInstaller.Catalog(r.Context())
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, catalog.Search(r.URL.Query().Get("query")))
}

func (s *Server) handlePluginTrust(w http.ResponseWriter, r *http.Request) {
	if !s.requireRoot(w, r) {
		return
	}
	var input pluginTrustRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Directory == "" {
		s.writeError(w, http.StatusBadRequest, "directory and trusted are required")
		return
	}
	state := s.pluginState
	if state.Root == "" {
		var err error
		state, err = pluginhost.DefaultState()
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := state.Trust(r.Context(), input.Directory, input.Trusted); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.scopes != nil {
		if err := s.scopes.ReloadPlugins(r.Context()); err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, api.StatusResponse{Status: "saved"})
}

func (s *Server) handlePluginCall(w http.ResponseWriter, r *http.Request) {
	var input pluginCallRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid plugin call")
		return
	}
	clientID := ""
	if !s.Ephemeral() {
		var err error
		clientID, err = s.resolveClientID(r)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	scope, release, err := s.executionScope(r.Context(), input.WorkingDirectory)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer release()
	identity := protocol.Identity{ClientID: clientID}
	if scope != nil {
		identity.WorkDir = scope.WorkDir()
	}
	activation, err := s.acquirePlugins(r.Context(), scope, identity)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if activation == nil {
		s.writeError(w, http.StatusNotFound, "plugin is not active")
		return
	}
	defer activation.Close(r.Context())
	var result json.RawMessage
	if err := activation.Invoke(r.Context(), input.Plugin, input.Method, input.Input, &result); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
