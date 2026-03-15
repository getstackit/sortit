package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"splat/internal/ai"
	"splat/internal/queries"
)

const debugAnalyzeTimeout = 45 * time.Second
const debugInvalidateMapProjectionTimeout = 15 * time.Second
const debugRescoreTagsTimeout = 120 * time.Second

type debugIssueAnalyzeRequest struct {
	Text string   `json:"text"`
	Tags []string `json:"tags,omitempty"`
}

type debugIssueSimilarity struct {
	ID         string   `json:"id"`
	Raw        string   `json:"raw"`
	Tags       []string `json:"tags"`
	Similarity float64  `json:"similarity"`
}

type debugIssueAnalyzeResponse struct {
	Tags                   []ai.TagScore          `json:"tags"`
	Embedding              ai.EmbeddingInfo       `json:"embedding"`
	Tagger                 ai.ModelInfo           `json:"tagger"`
	Embedder               ai.ModelInfo           `json:"embedder"`
	ComparedIssueCount     int                    `json:"comparedIssueCount"`
	AverageIssueSimilarity float64                `json:"averageIssueSimilarity"`
	SimilarIssues          []debugIssueSimilarity `json:"similarIssues"`
}

type debugInvalidateMapProjectionResponse struct {
	Invalidated bool `json:"invalidated"`
}

type debugRescoreTagsResponse struct {
	Rescored bool `json:"rescored"`
}

func (s *Server) handleDebugIssueAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	request, err := decodeDebugIssueAnalyzeRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), debugAnalyzeTimeout)
	defer cancel()

	analyzed, err := s.debugAnalyzeIssue.Handle(ctx, queries.DebugAnalyzeIssue{
		Text: request.Text,
		Tags: request.Tags,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if queries.AIUnavailable(err) {
			status = http.StatusServiceUnavailable
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, debugIssueAnalyzeResponse{
		Tags:                   analyzed.Tags,
		Embedding:              analyzed.Embedding,
		Tagger:                 analyzed.Tagger,
		Embedder:               analyzed.Embedder,
		ComparedIssueCount:     analyzed.ComparedIssueCount,
		AverageIssueSimilarity: analyzed.AverageIssueSimilarity,
		SimilarIssues:          toDebugIssueSimilarities(analyzed.SimilarIssues),
	})
}

func (s *Server) handleDebugInvalidateMapProjection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	invalidator := mapProjectionInvalidatorFromIssueStore(s.config.IssueStore)
	if invalidator == nil {
		writeError(w, http.StatusNotImplemented, "map projection invalidation is unavailable")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), debugInvalidateMapProjectionTimeout)
	defer cancel()

	if err := invalidator.InvalidateMapProjections(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, debugInvalidateMapProjectionResponse{Invalidated: true})
}

func (s *Server) handleDebugRescoreTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	if s.catalog == nil {
		writeError(w, http.StatusNotImplemented, "tag specificity scoring is unavailable")
		return
	}

	s.logger.InfoContext(r.Context(), "debug rescore tags requested")

	ctx, cancel := context.WithTimeout(r.Context(), debugRescoreTagsTimeout)
	defer cancel()

	start := time.Now()
	if err := s.catalog.ScoreAllTagsSpecificity(ctx); err != nil {
		writeInternalError(w, r, "failed to rescore tag specificity", err)
		return
	}

	s.revisions.Bump()

	s.logger.InfoContext(r.Context(), "debug rescore tags complete",
		"duration", time.Since(start).Round(time.Millisecond),
	)

	writeJSON(w, http.StatusOK, debugRescoreTagsResponse{Rescored: true})
}

func decodeDebugIssueAnalyzeRequest(r *http.Request) (debugIssueAnalyzeRequest, error) {
	request, err := decodeJSON[debugIssueAnalyzeRequest](r)
	if err != nil {
		return debugIssueAnalyzeRequest{}, err
	}

	request.Text = strings.TrimSpace(request.Text)
	if request.Text == "" {
		return debugIssueAnalyzeRequest{}, errors.New("text is required")
	}
	return request, nil
}

func toDebugIssueSimilarities(items []queries.DebugIssueSimilarity) []debugIssueSimilarity {
	out := make([]debugIssueSimilarity, len(items))
	for i, item := range items {
		out[i] = debugIssueSimilarity{
			ID:         item.ID,
			Raw:        item.Raw,
			Tags:       append([]string(nil), item.Tags...),
			Similarity: item.Similarity,
		}
	}
	return out
}
