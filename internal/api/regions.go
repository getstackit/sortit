package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"sortit/internal/auth"
	"sortit/internal/issues"
	"sortit/internal/regions"
)

func actorFromRequest(r *http.Request) string {
	if principal, ok := auth.PrincipalFromContext(r.Context()); ok {
		return principal.Login
	}
	return ""
}

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

func (s *Server) handleCustomRegionList(w http.ResponseWriter, r *http.Request) {
	out, err := s.customRegions.List(r.Context())
	if err != nil {
		s.writeCustomRegionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"regions": out})
}

func (s *Server) handleCustomRegionGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := s.customRegions.Get(r.Context(), id)
	if err != nil {
		s.writeCustomRegionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCustomRegionCreate(w http.ResponseWriter, r *http.Request) {
	input, err := decodeJSON[regions.CustomRegionInput](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	createdBy := actorFromRequest(r)
	out, err := s.customRegions.Create(r.Context(), input, createdBy)
	if err != nil {
		s.writeCustomRegionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleCustomRegionUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	input, err := decodeJSON[regions.CustomRegionInput](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	out, err := s.customRegions.Update(r.Context(), id, input)
	if err != nil {
		s.writeCustomRegionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCustomRegionDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.customRegions.Delete(r.Context(), id); err != nil {
		s.writeCustomRegionError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) writeCustomRegionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, issues.ErrCustomRegionNotFound):
		writeError(w, http.StatusNotFound, "custom region not found")
	case errors.Is(err, regions.ErrCustomStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
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
