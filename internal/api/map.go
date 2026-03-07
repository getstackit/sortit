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

	result, err := issuemap.BuildMap(viewport)
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

	result, err := issuemap.BuildEdgeResponse(viewport)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build edge response")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
