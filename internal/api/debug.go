package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"sortit/internal/ai"
	"sortit/internal/diagnostics"
	"sortit/internal/issues"
	"sortit/internal/tags"
)

type debugIssueAnalyzeRequest struct {
	Text          string   `json:"text"`
	Tags          []string `json:"tags,omitempty"`
	CandidateMode string   `json:"candidateMode,omitempty"`
	Verify        *bool    `json:"verify,omitempty"`
}

type debugIssueSimilarity struct {
	ID         string   `json:"id"`
	Raw        string   `json:"raw"`
	Tags       []string `json:"tags"`
	Similarity float64  `json:"similarity"`
}

type debugIssueAnalyzeResponse struct {
	Tags                   []issues.TagRelevance  `json:"tags"`
	CandidateSet           tags.CandidateTaxonomy `json:"candidateSet"`
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

	analyzed, err := s.debugAnalyzeIssue.Handle(r.Context(), diagnostics.DebugAnalyzeIssue{
		Text:          request.Text,
		Tags:          request.Tags,
		CandidateMode: tags.CandidateMode(request.CandidateMode),
		Verify:        request.Verify == nil || *request.Verify,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if diagnostics.AIUnavailable(err) {
			status = http.StatusServiceUnavailable
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, debugIssueAnalyzeResponse{
		Tags:                   analyzed.Tags,
		CandidateSet:           analyzed.CandidateSet,
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
	fixture := strings.TrimSpace(r.URL.Query().Get("fixture"))
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	verify := true
	if raw := strings.TrimSpace(r.URL.Query().Get("verify")); raw != "" {
		verify = !strings.EqualFold(raw, "false") && !strings.EqualFold(raw, "off") && raw != "0"
	}
	runs := 1
	if raw := strings.TrimSpace(r.URL.Query().Get("runs")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "runs must be a positive integer")
			return
		}
		runs = parsed
	}
	parsedMode, err := tags.ParseCandidateMode(mode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if parsedMode == tags.CandidateModeExplicitOnly {
		writeError(w, http.StatusBadRequest, "explicit-only candidate mode is not supported for benchmark evaluation")
		return
	}
	result, err := s.debugEvalTags.HandleFixture(r.Context(), fixture, runs, parsedMode, verify)
	if err != nil {
		status := http.StatusInternalServerError
		if diagnostics.AIUnavailable(err) {
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
	request.CandidateMode = strings.TrimSpace(request.CandidateMode)
	if request.Text == "" {
		return debugIssueAnalyzeRequest{}, errors.New("text is required")
	}
	mode, err := tags.ParseCandidateMode(request.CandidateMode)
	if err != nil {
		return debugIssueAnalyzeRequest{}, err
	}
	if mode == tags.CandidateModeExplicitOnly && len(request.Tags) == 0 {
		return debugIssueAnalyzeRequest{}, errors.New("explicit-only candidate mode requires at least one tag")
	}
	return request, nil
}

func toDebugIssueSimilarities(items []diagnostics.DebugIssueSimilarity) []debugIssueSimilarity {
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
