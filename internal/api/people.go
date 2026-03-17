package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"splat/internal/queries"
)

func (s *Server) handlePersonDetail(w http.ResponseWriter, r *http.Request) {
	person := chi.URLParam(r, "person")
	if person == "" {
		writeError(w, http.StatusBadRequest, "person name is required")
		return
	}

	detail, err := s.getPersonDetail.Handle(r.Context(), person)
	if err != nil {
		writeInternalError(w, r, "failed to get person detail", err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handlePersonProfileRoute(w http.ResponseWriter, r *http.Request) {
	person := chi.URLParam(r, "person")
	if person == "" {
		writeError(w, http.StatusBadRequest, "person name is required")
		return
	}

	filter, err := ParseIssueStatusFilter(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid status query")
		return
	}
	if r.URL.Query().Get("status") == "" {
		filter = queries.IssueStatusFilterAll
	}

	profile, err := s.getPersonProfile.Handle(r.Context(), person, filter)
	if err != nil {
		writeInternalError(w, r, "failed to get person profile", err)
		return
	}

	writeJSON(w, http.StatusOK, profile)
}

func (s *Server) handleWorkCorrelations(w http.ResponseWriter, r *http.Request) {
	filter, err := ParseIssueStatusFilter(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid status query")
		return
	}
	if r.URL.Query().Get("status") == "" {
		filter = queries.IssueStatusFilterAll
	}

	result, err := s.workCorrelations.Handle(r.Context(), filter)
	if err != nil {
		writeInternalError(w, r, "failed to get work correlations", err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}
