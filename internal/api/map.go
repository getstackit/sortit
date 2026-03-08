package api

import (
	"context"
	"net/http"

	"splat/internal/issues"
	issuemap "splat/internal/map"
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

	edgeThreshold, err := ParseEdgeThreshold(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid edge threshold query")
		return
	}

	storeIssues, err := s.issueStore.List(r.Context())
	if err != nil {
		writeInternalError(w, r, "failed to list issues", err)
		return
	}

	storeTags, err := s.listStoredTags(r.Context())
	if err != nil {
		writeInternalError(w, r, "failed to list tags", err)
		return
	}

	if edgeThreshold != nil {
		result, err := issuemap.BuildMapFromIssuesWithTagsAndThreshold(storeIssues, storeTags, viewport, *edgeThreshold)
		if err != nil {
			writeInternalError(w, r, "failed to build issue map", err)
			return
		}

		writeJSON(w, http.StatusOK, result)
		return
	}

	result, err := issuemap.BuildMapFromIssuesWithTags(storeIssues, storeTags, viewport)
	if err != nil {
		writeInternalError(w, r, "failed to build issue map", err)
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

	edgeThreshold, err := ParseEdgeThreshold(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid edge threshold query")
		return
	}

	storeIssues, err := s.issueStore.List(r.Context())
	if err != nil {
		writeInternalError(w, r, "failed to list issues", err)
		return
	}

	storeTags, err := s.listStoredTags(r.Context())
	if err != nil {
		writeInternalError(w, r, "failed to list tags", err)
		return
	}

	if edgeThreshold != nil {
		result, err := issuemap.BuildEdgeResponseFromIssuesWithTagsAndThreshold(storeIssues, storeTags, viewport, *edgeThreshold)
		if err != nil {
			writeInternalError(w, r, "failed to build edge response", err)
			return
		}

		writeJSON(w, http.StatusOK, result)
		return
	}

	result, err := issuemap.BuildEdgeResponseFromIssuesWithTags(storeIssues, storeTags, viewport)
	if err != nil {
		writeInternalError(w, r, "failed to build edge response", err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) listStoredTags(ctx context.Context) ([]issues.Tag, error) {
	if s.tagStore == nil {
		return nil, nil
	}
	return s.tagStore.ListTags(ctx)
}
