package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/models/catalog"
)

type CatalogDTO struct {
	Labs      []catalog.LabInfo       `json:"labs"`
	Models    []catalog.ModelMetadata `json:"models"`
	Providers []catalog.ProviderInfo  `json:"providers"`
	Routes    []catalog.RouteInfo     `json:"routes"`
}

func (s *Server) handleCatalogLabLogo(w http.ResponseWriter, r *http.Request) {
	logo, ok := catalog.LabLogo(chi.URLParam(r, "id"))
	if !ok {
		writeError(w, http.StatusNotFound, "lab logo not found")
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	_, _ = w.Write(logo)
}

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, CatalogDTO{
		Labs:      catalog.ListLabs(),
		Models:    catalog.ListCanonicalModels(),
		Providers: catalog.ListProviders(),
		Routes:    catalog.ListRoutes(),
	})
}
