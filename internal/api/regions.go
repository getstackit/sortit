package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"sortit/internal/regions"
)

func (s *Server) handleRegionsList(w http.ResponseWriter, r *http.Request) {
	result, err := s.regions.List(r.Context(), r.URL.Query().Get("kind"), r.URL.Query().Get("window"))
	if err != nil {
		s.writeRegionsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleRegionGet(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	id := chi.URLParam(r, "id")
	result, err := s.regions.Get(r.Context(), kind, id, r.URL.Query().Get("window"))
	if err != nil {
		s.writeRegionsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleRegionOrphans(w http.ResponseWriter, r *http.Request) {
	orphans, err := s.regions.Orphans(r.Context())
	if err != nil {
		s.writeRegionsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orphans)
}

func (s *Server) writeRegionsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, regions.ErrUnsupportedKind):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, regions.ErrRegionNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}
