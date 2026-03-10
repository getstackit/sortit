package api

import (
	"net/http"
	"strings"

	"splat/internal/queries"
)

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	limit, err := ParsePositiveIntQuery(r.URL.Query(), "limit")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit query")
		return
	}

	input := queries.ListActivity{
		Cursor: strings.TrimSpace(r.URL.Query().Get("cursor")),
		Kind:   strings.TrimSpace(r.URL.Query().Get("kind")),
	}
	if limit != nil {
		input.Limit = *limit
	}

	result, err := s.listActivity.Handle(r.Context(), input)
	if err != nil {
		writeInternalError(w, r, "failed to list activity", err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}
