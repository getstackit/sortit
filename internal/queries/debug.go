package queries

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"splat/internal/ai"
	"splat/internal/issues"
	issuemap "splat/internal/map"
	"splat/internal/services"
	"splat/internal/vectors"
)

type DebugAnalyzeIssue struct {
	Text string
	Tags []string
}

type DebugIssueSimilarity struct {
	ID         string
	Raw        string
	Tags       []string
	Similarity float64
}

type DebugAnalyzeIssueResult struct {
	Tags                   []ai.TagScore
	Embedding              ai.EmbeddingInfo
	Tagger                 ai.ModelInfo
	Embedder               ai.ModelInfo
	ComparedIssueCount     int
	AverageIssueSimilarity float64
	SimilarIssues          []DebugIssueSimilarity
}

type DebugAnalyzeIssueHandler struct {
	Analyzer *ai.Analyzer
	Catalog  *services.CatalogService
	Store    issues.Store
}

type issueEmbeddingSimilarityLister interface {
	ListIssueEmbeddingSimilarities(context.Context, []float64, int) ([]issues.IssueEmbeddingSimilarity, int, float64, error)
}

func (h DebugAnalyzeIssueHandler) Handle(ctx context.Context, input DebugAnalyzeIssue) (DebugAnalyzeIssueResult, error) {
	tags, err := h.Catalog.IssueTaxonomy(ctx, input.Tags)
	if err != nil {
		return DebugAnalyzeIssueResult{}, err
	}

	analyzed, err := h.Analyzer.AnalyzeIssueData(ctx, input.Text, tags)
	if err != nil {
		return DebugAnalyzeIssueResult{}, err
	}

	similarities, comparedIssueCount, averageSimilarity, err := h.issueEmbeddingSimilarities(ctx, services.Float32VectorToFloat64(analyzed.Embedding.Vector))
	if err != nil {
		return DebugAnalyzeIssueResult{}, err
	}

	return DebugAnalyzeIssueResult{
		Tags:                   analyzed.Tags,
		Embedding:              analyzed.Embedding.Info,
		Tagger:                 analyzed.Tagger,
		Embedder:               analyzed.Embedder,
		ComparedIssueCount:     comparedIssueCount,
		AverageIssueSimilarity: averageSimilarity,
		SimilarIssues:          similarities,
	}, nil
}

func (h DebugAnalyzeIssueHandler) issueEmbeddingSimilarities(ctx context.Context, query []float64) ([]DebugIssueSimilarity, int, float64, error) {
	if similarityStore, ok := h.Store.(issueEmbeddingSimilarityLister); ok {
		items, compared, average, err := similarityStore.ListIssueEmbeddingSimilarities(ctx, query, 8)
		if err != nil {
			return nil, 0, 0, err
		}
		if items != nil {
			similarities := make([]DebugIssueSimilarity, 0, len(items))
			for _, item := range items {
				similarities = append(similarities, DebugIssueSimilarity{
					ID:         item.ID,
					Raw:        item.Raw,
					Tags:       append([]string(nil), item.Tags...),
					Similarity: item.Similarity,
				})
			}
			return similarities, compared, average, nil
		}
	}

	var storeIssues []issues.Issue
	if h.Store != nil {
		items, err := h.Store.List(ctx)
		if err != nil {
			return nil, 0, 0, err
		}
		storeIssues = items
	}

	if len(query) == 0 {
		return []DebugIssueSimilarity{}, 0, 0, nil
	}

	comparisons := make([]DebugIssueSimilarity, 0, len(storeIssues))
	total := 0.0
	compared := 0
	for _, issue := range storeIssues {
		if len(issue.Embedding) == 0 {
			continue
		}

		similarity := vectors.CosineSimilarity(query, issue.Embedding)
		compared++
		total += similarity
		comparisons = append(comparisons, DebugIssueSimilarity{
			ID:         issue.ID,
			Raw:        issue.Raw,
			Tags:       append([]string(nil), issue.Tags...),
			Similarity: math.Round(similarity*100) / 100,
		})
	}

	slices.SortStableFunc(comparisons, func(a, b DebugIssueSimilarity) int {
		if c := cmp.Compare(b.Similarity, a.Similarity); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})

	average := 0.0
	if compared > 0 {
		average = math.Round((total/float64(compared))*100) / 100
	}
	if len(comparisons) > 8 {
		comparisons = comparisons[:8]
	}

	return comparisons, compared, average, nil
}

func AIUnavailable(err error) bool {
	return errors.Is(err, ai.ErrNotConfigured)
}

// DebugFactorWeightsResult holds the current factor decomposition weights
// and R² diagnostics computed from the issue corpus.
type DebugFactorWeightsResult struct {
	FactorWeight    float64           `json:"factorWeight"`
	ResidualWeight  float64           `json:"residualWeight"`
	AggregateR2     float64           `json:"aggregateR2"`
	IssueCount      int               `json:"issueCount"`
	DecomposedCount int               `json:"decomposedCount"`
	Decomposed      bool              `json:"decomposed"`
	LowR2Issues     []DebugLowR2Issue `json:"lowR2Issues"`
}

// DebugLowR2Issue identifies an issue poorly explained by the tag factor model.
type DebugLowR2Issue struct {
	ID   string   `json:"id"`
	Raw  string   `json:"raw"`
	R2   float64  `json:"r2"`
	Tags []string `json:"tags"`
}

type DebugFactorWeightsHandler struct {
	Store   issues.Store
	Catalog *services.CatalogService
}

func (h DebugFactorWeightsHandler) Handle(ctx context.Context) (DebugFactorWeightsResult, error) {
	var storeIssues []issues.Issue
	if h.Store != nil {
		items, err := h.Store.List(ctx)
		if err != nil {
			return DebugFactorWeightsResult{}, err
		}
		storeIssues = items
	}

	var storeTags []issues.Tag
	if h.Catalog != nil {
		tags, err := h.Catalog.StoredTags(ctx)
		if err == nil {
			storeTags = tags
		}
	}

	tagNames, tagEmbeddings := personDetailTagData(storeIssues, storeTags)
	issueEmbeddings := make(map[string][]float64, len(storeIssues))
	for _, issue := range storeIssues {
		if len(issue.Embedding) > 0 {
			issueEmbeddings[issue.ID] = issue.Embedding
		}
	}

	decomp := issuemap.ComputeFactorDecomposition(storeIssues, tagNames, issueEmbeddings, tagEmbeddings)

	// Build index for looking up issue data by ID.
	issueByID := make(map[string]issues.Issue, len(storeIssues))
	for _, issue := range storeIssues {
		issueByID[issue.ID] = issue
	}

	// Collect low-R² issues (below 0.15) sorted ascending — these are
	// candidates for new tags or re-classification.
	var lowR2 []DebugLowR2Issue
	decomp.AllR2(func(id string, r2 float64) {
		if r2 < 0.15 {
			issue := issueByID[id]
			tags := make([]string, 0, len(issue.TagScores))
			for _, ts := range issue.TagScores {
				tags = append(tags, ts.Tag)
			}
			lowR2 = append(lowR2, DebugLowR2Issue{
				ID:   id,
				Raw:  truncateRaw(issue.Raw, 120),
				R2:   math.Round(r2*1000) / 1000,
				Tags: tags,
			})
		}
	})
	slices.SortFunc(lowR2, func(a, b DebugLowR2Issue) int {
		return cmp.Compare(a.R2, b.R2)
	})
	if len(lowR2) > 20 {
		lowR2 = lowR2[:20]
	}

	return DebugFactorWeightsResult{
		FactorWeight:    math.Round(decomp.FactorWeight*1000) / 1000,
		ResidualWeight:  math.Round(decomp.ResidualWeight*1000) / 1000,
		AggregateR2:     math.Round(decomp.AggregateR2*1000) / 1000,
		IssueCount:      len(storeIssues),
		DecomposedCount: decomp.DecomposedCount(),
		Decomposed:      decomp.Decomposed(),
		LowR2Issues:     lowR2,
	}, nil
}

func truncateRaw(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// DebugIssueR2Result provides a detailed factor decomposition diagnostic
// for a single issue: what the tags explain, what they don't, and which
// catalog tags are closest to the unexplained residual.
type DebugIssueR2Result struct {
	ID                  string                  `json:"id"`
	Raw                 string                  `json:"raw"`
	R2                  float64                 `json:"r2"`
	TagCount            int                     `json:"tagCount"`
	Tags                []DebugIssueR2Tag       `json:"tags"`
	EmbeddingDim        int                     `json:"embeddingDim"`
	FactorAlignment     float64                 `json:"factorAlignment"`
	ResidualNorm        float64                 `json:"residualNorm"`
	NearestResidualTags []DebugResidualTagMatch `json:"nearestResidualTags"`
	ResidualNeighbors   []DebugResidualNeighbor `json:"residualNeighbors"`
	Diagnosis           []string                `json:"diagnosis"`
	Skipped             bool                    `json:"skipped"`
	SkipReason          string                  `json:"skipReason,omitempty"`
}

// DebugResidualNeighbor is an issue whose residual points in a similar
// direction — they share unexplained concepts. A cluster of these is a
// strong signal for a missing tag.
type DebugResidualNeighbor struct {
	ID         string   `json:"id"`
	Raw        string   `json:"raw"`
	Similarity float64  `json:"similarity"`
	R2         float64  `json:"r2"`
	Tags       []string `json:"tags"`
}

// DebugIssueR2Tag shows a tag assignment and its contribution to the factor model.
type DebugIssueR2Tag struct {
	Tag       string  `json:"tag"`
	Relevance float64 `json:"relevance"`
	// Alignment is the cosine similarity between this tag's embedding and the
	// issue's embedding. High alignment means this tag "points in the same
	// direction" as the issue.
	Alignment float64 `json:"alignment"`
}

// DebugResidualTagMatch shows a catalog tag that is close to the unexplained
// residual — a candidate tag that could improve R² if assigned.
type DebugResidualTagMatch struct {
	Tag        string  `json:"tag"`
	Similarity float64 `json:"similarity"`
	Assigned   bool    `json:"assigned"`
}

type DebugIssueR2Handler struct {
	Store   issues.Store
	Catalog *services.CatalogService
}

func (h DebugIssueR2Handler) Handle(ctx context.Context, issueID string) (DebugIssueR2Result, error) {
	if h.Store == nil {
		return DebugIssueR2Result{}, errors.New("store is not available")
	}

	allIssues, err := h.Store.List(ctx)
	if err != nil {
		return DebugIssueR2Result{}, err
	}

	// Find the target issue.
	var target issues.Issue
	found := false
	for _, issue := range allIssues {
		if issue.ID == issueID {
			target = issue
			found = true
			break
		}
	}
	if !found {
		return DebugIssueR2Result{}, issues.ErrNotFound
	}

	var storeTags []issues.Tag
	if h.Catalog != nil {
		storeTags, _ = h.Catalog.StoredTags(ctx)
	}

	tagNames, tagEmbeddings := personDetailTagData(allIssues, storeTags)
	issueEmbeddings := make(map[string][]float64, len(allIssues))
	for _, issue := range allIssues {
		if len(issue.Embedding) > 0 {
			issueEmbeddings[issue.ID] = issue.Embedding
		}
	}

	result := DebugIssueR2Result{
		ID:  target.ID,
		Raw: target.Raw,
	}

	targetEmb := issueEmbeddings[target.ID]
	if len(targetEmb) == 0 {
		result.Skipped = true
		result.SkipReason = "no embedding stored for this issue"
		return result, nil
	}
	result.EmbeddingDim = len(targetEmb)

	// Determine tag embedding dimension.
	embDim := 0
	for _, emb := range tagEmbeddings {
		if len(emb) > 0 {
			embDim = len(emb)
			break
		}
	}
	if embDim == 0 || len(targetEmb) != embDim {
		result.Skipped = true
		result.SkipReason = fmt.Sprintf("dimension mismatch: issue=%d, tags=%d", len(targetEmb), embDim)
		return result, nil
	}

	// Build per-tag alignment with the issue embedding.
	assignedTags := make(map[string]struct{}, len(target.TagScores))
	for _, ts := range target.TagScores {
		alignment := 0.0
		if tagEmb, ok := tagEmbeddings[ts.Tag]; ok && len(tagEmb) == embDim {
			alignment = vectors.CosineSimilarity(targetEmb, tagEmb)
		}
		result.Tags = append(result.Tags, DebugIssueR2Tag{
			Tag:       ts.Tag,
			Relevance: ts.Relevance,
			Alignment: math.Round(alignment*1000) / 1000,
		})
		assignedTags[ts.Tag] = struct{}{}
	}
	result.TagCount = len(result.Tags)

	// Compute this issue's decomposition.
	decomp := issuemap.ComputeFactorDecomposition(allIssues, tagNames, issueEmbeddings, tagEmbeddings)

	r2, ok := decomp.IssueR2(target.ID)
	if !ok {
		result.Skipped = true
		result.SkipReason = "issue was not included in decomposition"
		return result, nil
	}
	result.R2 = math.Round(r2*1000) / 1000

	// Factor alignment = cosine similarity between issue embedding and
	// factor-predicted embedding (before normalization). We recompute from
	// the decomp outputs: factorEmb is unit-length, so dot(issueEmb, factorEmb)
	// gives cosine similarity if issueEmb is also unit-length.
	if factorEmb := decomp.FactorEmbedding(target.ID); len(factorEmb) > 0 {
		result.FactorAlignment = math.Round(vectors.CosineSimilarity(targetEmb, factorEmb)*1000) / 1000
	}
	if residualEmb := decomp.ResidualEmbedding(target.ID); len(residualEmb) > 0 {
		result.ResidualNorm = math.Round(math.Sqrt(dotProduct64(residualEmb, residualEmb))*1000) / 1000
	}

	// Find catalog tags closest to the residual embedding — these are
	// concepts the current tags don't capture.
	residualEmb := decomp.ResidualEmbedding(target.ID)
	if len(residualEmb) > 0 {
		type tagSim struct {
			tag      string
			sim      float64
			assigned bool
		}
		var candidates []tagSim
		for _, name := range tagNames {
			emb := tagEmbeddings[name]
			if len(emb) != embDim {
				continue
			}
			sim := vectors.CosineSimilarity(residualEmb, emb)
			_, assigned := assignedTags[name]
			candidates = append(candidates, tagSim{tag: name, sim: sim, assigned: assigned})
		}
		slices.SortFunc(candidates, func(a, b tagSim) int {
			return cmp.Compare(b.sim, a.sim)
		})
		limit := min(10, len(candidates))
		for _, c := range candidates[:limit] {
			result.NearestResidualTags = append(result.NearestResidualTags, DebugResidualTagMatch{
				Tag:        c.tag,
				Similarity: math.Round(c.sim*1000) / 1000,
				Assigned:   c.assigned,
			})
		}

		// Build index for neighbor lookup.
		issueByID := make(map[string]issues.Issue, len(allIssues))
		for _, issue := range allIssues {
			issueByID[issue.ID] = issue
		}

		// Find other issues whose residuals point in a similar direction.
		// These share whatever concept the tags are missing.
		type neighborSim struct {
			id  string
			sim float64
		}
		var neighbors []neighborSim
		for _, issue := range allIssues {
			if issue.ID == target.ID {
				continue
			}
			otherResidual := decomp.ResidualEmbedding(issue.ID)
			if len(otherResidual) == 0 {
				continue
			}
			sim := vectors.CosineSimilarity(residualEmb, otherResidual)
			if sim > 0.3 {
				neighbors = append(neighbors, neighborSim{id: issue.ID, sim: sim})
			}
		}
		slices.SortFunc(neighbors, func(a, b neighborSim) int {
			return cmp.Compare(b.sim, a.sim)
		})
		neighborLimit := min(8, len(neighbors))
		for _, n := range neighbors[:neighborLimit] {
			issue := issueByID[n.id]
			tags := make([]string, 0, len(issue.TagScores))
			for _, ts := range issue.TagScores {
				tags = append(tags, ts.Tag)
			}
			neighborR2, _ := decomp.IssueR2(n.id)
			result.ResidualNeighbors = append(result.ResidualNeighbors, DebugResidualNeighbor{
				ID:         n.id,
				Raw:        truncateRaw(issue.Raw, 120),
				Similarity: math.Round(n.sim*1000) / 1000,
				R2:         math.Round(neighborR2*1000) / 1000,
				Tags:       tags,
			})
		}
	}

	// Generate human-readable diagnosis.
	result.Diagnosis = diagnoseR2(result)

	return result, nil
}

// issueByID is used via closure in the Handle method above — this is a
// standalone helper for the diagnosis.
func diagnoseR2(r DebugIssueR2Result) []string {
	var diagnosis []string

	if r.R2 < 0.1 {
		diagnosis = append(diagnosis, "Tags explain almost none of this issue's embedding. The issue likely contains concepts the taxonomy doesn't cover.")
	} else if r.R2 < 0.2 {
		diagnosis = append(diagnosis, "Tags explain very little of this issue's embedding.")
	}

	if r.TagCount == 0 {
		diagnosis = append(diagnosis, "No tags assigned. Re-enrich this issue or check why the tagger returned nothing.")
		return diagnosis
	}

	// Check for low-alignment assigned tags.
	var lowAlignTags []string
	for _, t := range r.Tags {
		if t.Relevance >= 0.3 && t.Alignment < 0.1 {
			lowAlignTags = append(lowAlignTags, fmt.Sprintf("%s (relevance=%.2f, alignment=%.3f)", t.Tag, t.Relevance, t.Alignment))
		}
	}
	if len(lowAlignTags) > 0 {
		diagnosis = append(diagnosis, fmt.Sprintf("Potentially misclassified tags — high relevance but low alignment with the issue embedding: %s", strings.Join(lowAlignTags, ", ")))
	}

	// Check for unassigned tags close to the residual.
	var unassignedClose []string
	for _, rt := range r.NearestResidualTags {
		if !rt.Assigned && rt.Similarity > 0.2 {
			unassignedClose = append(unassignedClose, fmt.Sprintf("%s (%.3f)", rt.Tag, rt.Similarity))
		}
	}
	if len(unassignedClose) > 0 {
		diagnosis = append(diagnosis, fmt.Sprintf("Existing catalog tags close to the residual but not assigned — re-enrichment may help: %s", strings.Join(unassignedClose, ", ")))
	}

	// Check if residual neighbors form a cluster.
	if len(r.ResidualNeighbors) >= 3 {
		lowR2Count := 0
		for _, n := range r.ResidualNeighbors {
			if n.R2 < 0.15 {
				lowR2Count++
			}
		}
		if lowR2Count >= 2 {
			diagnosis = append(diagnosis, fmt.Sprintf("%d issues share a similar unexplained residual, and %d of them also have low R². This cluster likely represents a missing tag concept.", len(r.ResidualNeighbors), lowR2Count))
		} else {
			diagnosis = append(diagnosis, fmt.Sprintf("%d issues share a similar residual direction. Read them to identify the common concept the taxonomy is missing.", len(r.ResidualNeighbors)))
		}
	}

	// Check if no catalog tag is close to the residual at all.
	if len(r.NearestResidualTags) > 0 {
		topResidualSim := r.NearestResidualTags[0].Similarity
		if topResidualSim < 0.15 {
			diagnosis = append(diagnosis, "No existing catalog tag is close to the residual. This issue may need a completely new tag concept.")
		}
	}

	if len(diagnosis) == 0 {
		diagnosis = append(diagnosis, "No obvious issues detected. The low R² may reflect genuinely novel language not yet captured by any tag.")
	}

	return diagnosis
}

func dotProduct64(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var sum float64
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}
