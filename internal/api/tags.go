package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"splat/internal/issues"
)

type tagsResponse struct {
	Tags []tagResponse `json:"tags"`
}

type tagResponse struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	Embedding   []float64 `json:"embedding,omitempty"`
}

type mergeTagsRequest struct {
	Canonical string   `json:"canonical"`
	Aliases   []string `json:"aliases"`
}

type dismissTagMergeRequest struct {
	Canonical string `json:"canonical"`
	Alias     string `json:"alias"`
}

type dismissedTagMergeResponse struct {
	CanonicalName string `json:"canonicalName"`
	AliasName     string `json:"aliasName"`
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	tags, err := s.listTags.Handle(r.Context())
	if err != nil {
		writeInternalError(w, r, "failed to list tags", err)
		return
	}

	writeJSON(w, http.StatusOK, tagsResponse{Tags: toTagResponses(tags)})
}

func (s *Server) handleTagMerge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	request, err := decodeMergeTagsRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	merger := tagMergerFromIssueStore(s.config.IssueStore)
	if merger == nil {
		writeError(w, http.StatusNotImplemented, "tag merging is not supported by this store")
		return
	}

	if err := merger.MergeTags(r.Context(), request.Canonical, request.Aliases); err != nil {
		writeInternalError(w, r, "failed to merge tags", err)
		return
	}

	s.revisions.Bump()

	// Rescore the canonical tag's specificity after merge.
	if s.catalog != nil {
		if err := s.catalog.ScoreTagSpecificity(r.Context(), request.Canonical); err != nil {
			s.logger.WarnContext(r.Context(), "failed to rescore tag specificity after merge",
				"tag", request.Canonical,
				"error", err,
			)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "merged"})
}

func (s *Server) handleTagDismiss(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	request, err := decodeDismissTagMergeRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	merger := tagMergerFromIssueStore(s.config.IssueStore)
	if merger == nil {
		writeError(w, http.StatusNotImplemented, "tag merge dismissal is not supported by this store")
		return
	}

	if err := merger.DismissTagMerge(r.Context(), request.Canonical, request.Alias); err != nil {
		writeInternalError(w, r, "failed to dismiss tag merge", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "dismissed"})
}

func (s *Server) handleTagDismissedList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	merger := tagMergerFromIssueStore(s.config.IssueStore)
	if merger == nil {
		writeJSON(w, http.StatusOK, map[string]any{"dismissed": []any{}})
		return
	}

	items, err := merger.ListDismissedTagMerges(r.Context())
	if err != nil {
		writeInternalError(w, r, "failed to list dismissed tag merges", err)
		return
	}

	out := make([]dismissedTagMergeResponse, len(items))
	for i, item := range items {
		out[i] = dismissedTagMergeResponse{
			CanonicalName: item.CanonicalName,
			AliasName:     item.AliasName,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"dismissed": out})
}

func tagMergerFromIssueStore(store issues.Store) issues.TagMerger {
	merger, ok := store.(issues.TagMerger)
	if !ok {
		return nil
	}
	return merger
}

func decodeMergeTagsRequest(r *http.Request) (mergeTagsRequest, error) {
	request, err := decodeJSON[mergeTagsRequest](r)
	if err != nil {
		return mergeTagsRequest{}, err
	}

	request.Canonical = strings.TrimSpace(request.Canonical)
	if request.Canonical == "" {
		return mergeTagsRequest{}, errors.New("canonical tag name is required")
	}

	cleaned := make([]string, 0, len(request.Aliases))
	for _, alias := range request.Aliases {
		alias = strings.TrimSpace(alias)
		if alias != "" {
			cleaned = append(cleaned, alias)
		}
	}
	if len(cleaned) == 0 {
		return mergeTagsRequest{}, errors.New("at least one alias is required")
	}
	request.Aliases = cleaned
	return request, nil
}

func decodeDismissTagMergeRequest(r *http.Request) (dismissTagMergeRequest, error) {
	request, err := decodeJSON[dismissTagMergeRequest](r)
	if err != nil {
		return dismissTagMergeRequest{}, err
	}

	request.Canonical = strings.TrimSpace(request.Canonical)
	request.Alias = strings.TrimSpace(request.Alias)
	if request.Canonical == "" || request.Alias == "" {
		return dismissTagMergeRequest{}, errors.New("canonical and alias tag names are required")
	}
	return request, nil
}

func toTagResponses(tags []issues.Tag) []tagResponse {
	items := make([]tagResponse, len(tags))
	for i, tag := range tags {
		items[i] = tagResponse{
			Name:        tag.Name,
			Description: tag.Description,
			CreatedAt:   tag.CreatedAt,
			Embedding:   append([]float64(nil), tag.Embedding...),
		}
	}
	return items
}
