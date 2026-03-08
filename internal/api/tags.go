package api

import (
	"context"
	"net/http"
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

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	tags, err := s.availableTags(r.Context())
	if err != nil {
		writeInternalError(w, r, "failed to list tags", err)
		return
	}

	writeJSON(w, http.StatusOK, tagsResponse{Tags: toTagResponses(tags)})
}

func (s *Server) availableTags(ctx context.Context) ([]issues.Tag, error) {
	tags, err := s.listStoredTags(ctx)
	if err != nil {
		return nil, err
	}
	if len(tags) > 0 {
		return tags, nil
	}
	return issues.DefaultTags(), nil
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
