package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"sortit/internal/issues"
	"sortit/internal/issuexray"
)

func (s *Server) handleIssueXRay(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing issue id")
		return
	}
	result, err := s.issueXRay.Get(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, issuexray.ErrIssueNotFound), errors.Is(err, issues.ErrNotFound):
			writeError(w, http.StatusNotFound, "issue not found")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}
