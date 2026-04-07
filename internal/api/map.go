package api

import (
	"net/http"

	"splat/internal/mapview"
)

func (s *Server) handleMap(w http.ResponseWriter, r *http.Request) {
	viewport, err := ParseViewport(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid viewport query")
		return
	}

	edgeThreshold, err := ParseEdgeThreshold(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid edge threshold query")
		return
	}
	statusFilter, err := ParseIssueStatusFilter(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid status query")
		return
	}

	result, err := s.getMap.Handle(r.Context(), mapview.MapQuery{
		Viewport:      viewport,
		EdgeThreshold: edgeThreshold,
		StatusFilter:  statusFilter,
	})
	if err != nil {
		writeInternalError(w, r, "failed to build issue map", err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMapEdges(w http.ResponseWriter, r *http.Request) {
	viewport, err := ParseViewport(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid viewport query")
		return
	}

	edgeThreshold, err := ParseEdgeThreshold(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid edge threshold query")
		return
	}
	statusFilter, err := ParseIssueStatusFilter(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid status query")
		return
	}

	result, err := s.getMapEdges.Handle(r.Context(), mapview.MapQuery{
		Viewport:      viewport,
		EdgeThreshold: edgeThreshold,
		StatusFilter:  statusFilter,
	})
	if err != nil {
		writeInternalError(w, r, "failed to build edge response", err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}
