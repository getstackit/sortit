package api

import (
	"net/http"
	"strings"

	"splat/internal/queries"
)

func (s *Server) handleUnifiedSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

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

	result, err := s.searchUnified.Handle(r.Context(), queries.SearchUnified{
		Query: query,
		Limit: searchLimit,
	})
	if err != nil {
		writeInternalError(w, r, "failed to search", err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}
