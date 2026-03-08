package api

import (
	"net/http"
	"strings"

	"splat/internal/queries"
)

func (s *Server) handlePersonProfile(route string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		tail := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, route))
		segments := strings.Split(tail, "/")

		if len(segments) != 2 || segments[1] != "profile" {
			writeError(w, http.StatusNotFound, "route not found")
			return
		}

		person := strings.TrimSpace(segments[0])
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
}

func (s *Server) handleWorkCorrelations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
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

	result, err := s.workCorrelations.Handle(r.Context(), filter)
	if err != nil {
		writeInternalError(w, r, "failed to get work correlations", err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}
