package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"splat/internal/ai"
	"splat/internal/issues"
)

const debugAnalyzeTimeout = 45 * time.Second

type debugIssueAnalyzeRequest struct {
	Text string   `json:"text"`
	Tags []string `json:"tags,omitempty"`
}

type replaceableIssueStore interface {
	Replace(context.Context, []issues.Issue) error
}

type debugIssueStoreResponse struct {
	IssueCount int `json:"issueCount"`
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

	tags, err := s.issueTaxonomy(ctx, request.Tags)
	if err != nil {
		writeInternalError(w, r, "failed to load tag taxonomy", err)
		return
	}

	analyzed, err := s.analyzer.AnalyzeIssueData(ctx, request.Text, tags)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ai.ErrNotConfigured) {
			status = http.StatusServiceUnavailable
		}
		writeError(w, status, err.Error())
		return
	}

	similarities, comparedIssueCount, averageSimilarity, err := s.issueEmbeddingSimilarities(ctx, float32VectorToFloat64(analyzed.Embedding.Vector))
	if err != nil {
		writeInternalError(w, r, "failed to compute issue similarities", err)
		return
	}

	writeJSON(w, http.StatusOK, debugIssueAnalyzeResponse{
		Tags:                   analyzed.Tags,
		Embedding:              analyzed.Embedding.Info,
		Tagger:                 analyzed.Tagger,
		Embedder:               analyzed.Embedder,
		ComparedIssueCount:     comparedIssueCount,
		AverageIssueSimilarity: averageSimilarity,
		SimilarIssues:          similarities,
	})
}

func (s *Server) handleDebugIssueSampleLoad(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	store, ok := s.issueStore.(replaceableIssueStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "issue store cannot be reset")
		return
	}

	samples := issues.SeedIssues()
	enriched, err := s.analyzeSeedIssues(r.Context(), samples)
	if err != nil {
		writeInternalError(w, r, "failed to analyze sample issues", err)
		return
	}

	if err := store.Replace(r.Context(), enriched); err != nil {
		writeInternalError(w, r, "failed to load sample issues", err)
		return
	}

	writeJSON(w, http.StatusOK, debugIssueStoreResponse{IssueCount: len(samples)})
}

func (s *Server) handleDebugIssueReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	store, ok := s.issueStore.(replaceableIssueStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "issue store cannot be reset")
		return
	}

	if err := store.Replace(r.Context(), nil); err != nil {
		writeInternalError(w, r, "failed to clear issues", err)
		return
	}

	writeJSON(w, http.StatusOK, debugIssueStoreResponse{IssueCount: 0})
}

func decodeDebugIssueAnalyzeRequest(r *http.Request) (debugIssueAnalyzeRequest, error) {
	defer r.Body.Close()

	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()

	var request debugIssueAnalyzeRequest
	if err := decoder.Decode(&request); err != nil {
		return debugIssueAnalyzeRequest{}, errors.New("invalid request body")
	}

	request.Text = strings.TrimSpace(request.Text)
	if request.Text == "" {
		return debugIssueAnalyzeRequest{}, errors.New("text is required")
	}
	return request, nil
}

func (s *Server) issueEmbeddingSimilarities(ctx context.Context, query []float64) ([]debugIssueSimilarity, int, float64, error) {
	storeIssues, err := s.issueStore.List(ctx)
	if err != nil {
		return nil, 0, 0, err
	}

	if len(query) == 0 {
		return []debugIssueSimilarity{}, 0, 0, nil
	}

	comparisons := make([]debugIssueSimilarity, 0, len(storeIssues))
	total := 0.0
	compared := 0
	for _, issue := range storeIssues {
		if len(issue.Embedding) == 0 {
			continue
		}

		similarity := cosineSimilarity(query, issue.Embedding)
		compared++
		total += similarity
		comparisons = append(comparisons, debugIssueSimilarity{
			ID:         issue.ID,
			Raw:        issue.Raw,
			Tags:       append([]string(nil), issue.Tags...),
			Similarity: math.Round(similarity*100) / 100,
		})
	}

	sort.Slice(comparisons, func(i, j int) bool {
		if comparisons[i].Similarity == comparisons[j].Similarity {
			return comparisons[i].ID < comparisons[j].ID
		}
		return comparisons[i].Similarity > comparisons[j].Similarity
	})

	average := 0.0
	if compared > 0 {
		average = total / float64(compared)
		average = math.Round(average*100) / 100
	}
	if len(comparisons) > 8 {
		comparisons = comparisons[:8]
	}

	return comparisons, compared, average, nil
}
