package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"splat/internal/ai"
	"splat/internal/issues"
	"splat/internal/queries"
)

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
	request, err := decodeDebugIssueAnalyzeRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	analyzed, err := s.debugAnalyzeIssue.Handle(r.Context(), queries.DebugAnalyzeIssue{
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
	invalidator := mapProjectionInvalidatorFromIssueStore(s.config.IssueStore)
	if invalidator == nil {
		writeError(w, http.StatusNotImplemented, "map projection invalidation is unavailable")
		return
	}

	if err := invalidator.InvalidateMapProjections(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, debugInvalidateMapProjectionResponse{Invalidated: true})
}

func (s *Server) handleDebugRescoreTags(w http.ResponseWriter, r *http.Request) {
	if s.catalog == nil {
		writeError(w, http.StatusNotImplemented, "tag specificity scoring is unavailable")
		return
	}

	s.logger.InfoContext(r.Context(), "debug rescore tags requested")

	start := time.Now()
	if err := s.catalog.ScoreAllTagsSpecificity(r.Context()); err != nil {
		writeInternalError(w, r, "failed to rescore tag specificity", err)
		return
	}

	s.revisions.Bump()

	s.logger.InfoContext(r.Context(), "debug rescore tags complete",
		"duration", time.Since(start).Round(time.Millisecond),
	)

	writeJSON(w, http.StatusOK, debugRescoreTagsResponse{Rescored: true})
}

func (s *Server) handleDebugEvalTags(w http.ResponseWriter, r *http.Request) {
	result, err := s.debugEvalTags.Handle(r.Context())
	if err != nil {
		status := http.StatusInternalServerError
		if queries.AIUnavailable(err) {
			status = http.StatusServiceUnavailable
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleDebugIssueR2(w http.ResponseWriter, r *http.Request) {
	issueID := strings.TrimSpace(chi.URLParam(r, "id"))
	if issueID == "" {
		writeError(w, http.StatusBadRequest, "issue id is required")
		return
	}

	result, err := s.debugIssueR2.Handle(r.Context(), issueID)
	if err != nil {
		if errors.Is(err, issues.ErrNotFound) {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleDebugFactorWeights(w http.ResponseWriter, r *http.Request) {
	result, err := s.debugFactorWeights.Handle(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
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
