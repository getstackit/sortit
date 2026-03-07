package api

import (
	"net/http"

	issuemap "bored/internal/map"
)

func (s *Server) handleMap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	viewport, err := ParseViewport(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid viewport query")
		return
	}

	storeIssues, err := s.issueStore.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list issues")
		return
	}

	result, err := issuemap.BuildMapFromIssues(storeIssues, viewport)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build issue map")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMapEdges(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	viewport, err := ParseViewport(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid viewport query")
		return
	}

	storeIssues, err := s.issueStore.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list issues")
		return
	}

	result, err := issuemap.BuildEdgeResponseFromIssues(storeIssues, viewport)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build edge response")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
