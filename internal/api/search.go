package api

import (
	"net/http"
	"strings"

	"sortit/internal/search"
)

func (s *Server) handleUnifiedSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		query = strings.TrimSpace(r.URL.Query().Get("query"))
	}
	if query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}

	limit, err := ParsePositiveIntQuery(r.URL.Query(), "limit")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit query")
		return
	}

	searchLimit := 10
	if limit != nil {
		searchLimit = *limit
	}

	result, err := s.searchUnified.Handle(r.Context(), search.SearchUnified{
		Query: query,
		Limit: searchLimit,
	})
	if err != nil {
		writeInternalError(w, r, "failed to search", err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}
