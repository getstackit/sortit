package api

import (
	"net/http"
	"strings"

	issueviews "sortit/internal/issues/views"
)

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	limit, err := ParsePositiveIntQuery(r.URL.Query(), "limit")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit query")
		return
	}

	input := issueviews.ListActivity{
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
