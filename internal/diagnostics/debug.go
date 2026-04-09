package diagnostics

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"sortit/internal/ai"
	issueenrichment "sortit/internal/issueenrichment"
	"sortit/internal/issuemath"
	"sortit/internal/issues"
	"sortit/internal/tags"
	"sortit/internal/vectors"
)

const debugIssueRawPreviewMaxLen = 120

type DebugAnalyzeIssue struct {
	Text          string
	Tags          []string
	CandidateMode tags.CandidateMode
	Verify        bool
}

type DebugIssueSimilarity struct {
	ID         string
	Raw        string
	Tags       []string
	Similarity float64
}

type DebugAnalyzeIssueResult struct {
	Tags                   []issues.TagRelevance
	CandidateSet           tags.CandidateTaxonomy
	Embedding              ai.EmbeddingInfo
	Tagger                 ai.ModelInfo
	Embedder               ai.ModelInfo
	ComparedIssueCount     int
	AverageIssueSimilarity float64
	SimilarIssues          []DebugIssueSimilarity
}

type DebugAnalyzeIssueHandler struct {
	Analyzer *ai.Analyzer
	Catalog  *tags.CatalogService
	Enricher *issueenrichment.IssueEnricher
	Store    issues.Store
}

type issueEmbeddingSimilarityLister interface {
	ListIssueEmbeddingSimilarities(context.Context, []float64, int) ([]issues.IssueEmbeddingSimilarity, int, float64, error)
}

func (h DebugAnalyzeIssueHandler) Handle(ctx context.Context, input DebugAnalyzeIssue) (DebugAnalyzeIssueResult, error) {
	mode, err := tags.ParseCandidateMode(string(input.CandidateMode))
	if err != nil {
		return DebugAnalyzeIssueResult{}, err
	}
	if len(input.Tags) > 0 {
		mode = tags.CandidateModeExplicitOnly
	}
	if h.Enricher == nil {
		return DebugAnalyzeIssueResult{}, errors.New("issue enricher is not available")
	}
	analysis, err := h.Enricher.AnalyzeText(ctx, input.Text, issueenrichment.AnalyzeTextOptions{
		PreferredTags: input.Tags,
		CandidateMode: mode,
		Verify:        input.Verify,
	})
	if err != nil {
		return DebugAnalyzeIssueResult{}, err
	}

	similarities, comparedIssueCount, averageSimilarity, err := h.issueEmbeddingSimilarities(ctx, issueenrichment.Float32VectorToFloat64(analysis.Analyzed.Embedding.Vector))
	if err != nil {
		return DebugAnalyzeIssueResult{}, err
	}

	return DebugAnalyzeIssueResult{
		Tags:                   analysis.TagScores,
		CandidateSet:           analysis.CandidateSet,
		Embedding:              analysis.Analyzed.Embedding.Info,
		Tagger:                 analysis.Analyzed.Tagger,
		Embedder:               analysis.Analyzed.Embedder,
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
	ReviewQueue     DebugReviewQueue  `json:"reviewQueue"`
}

// DebugLowR2Issue identifies an issue poorly explained by the tag factor model.
type DebugLowR2Issue struct {
	ID     string             `json:"id"`
	Raw    string             `json:"raw"`
	Status issues.IssueStatus `json:"status"`
	R2     float64            `json:"r2"`
	Tags   []string           `json:"tags"`
}

type DebugReviewQueue struct {
	LowR2Issues      []DebugReviewIssue        `json:"lowR2Issues"`
	LowAlignmentTags []DebugReviewLowAlignment `json:"lowAlignmentTags"`
	ResidualMisses   []DebugReviewResidualMiss `json:"residualMisses"`
}

type DebugReviewIssue struct {
	ID        string                `json:"id"`
	Raw       string                `json:"raw"`
	Status    issues.IssueStatus    `json:"status"`
	R2        float64               `json:"r2"`
	TagScores []issues.TagRelevance `json:"tagScores"`
	Diagnosis []string              `json:"diagnosis"`
}

type DebugReviewLowAlignment struct {
	IssueID     string              `json:"issueId"`
	IssueRaw    string              `json:"issueRaw"`
	IssueStatus issues.IssueStatus  `json:"issueStatus"`
	IssueR2     float64             `json:"issueR2"`
	TagScore    issues.TagRelevance `json:"tagScore"`
	Alignment   float64             `json:"alignment"`
}

type DebugReviewResidualMiss struct {
	IssueID       string                  `json:"issueId"`
	IssueRaw      string                  `json:"issueRaw"`
	IssueStatus   issues.IssueStatus      `json:"issueStatus"`
	IssueR2       float64                 `json:"issueR2"`
	TagScores     []issues.TagRelevance   `json:"tagScores"`
	CandidateTags []DebugResidualTagMatch `json:"candidateTags"`
}

type DebugFactorWeightsHandler struct {
	Store   issues.Store
	Catalog *tags.CatalogService
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

	tagNames, tagEmbeddings := tagDataFromIssues(storeIssues, storeTags)
	issueEmbeddings := make(map[string][]float64, len(storeIssues))
	for _, issue := range storeIssues {
		if len(issue.Embedding) > 0 {
			issueEmbeddings[issue.ID] = issue.Embedding
		}
	}

	decomp := issuemath.ComputeFactorDecomposition(storeIssues, tagNames, issueEmbeddings, tagEmbeddings)

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
				ID:     id,
				Raw:    truncateRaw(issue.Raw),
				Status: issue.Status,
				R2:     math.Round(r2*1000) / 1000,
				Tags:   tags,
			})
		}
	})
	slices.SortFunc(lowR2, func(a, b DebugLowR2Issue) int {
		return cmp.Compare(a.R2, b.R2)
	})
	if len(lowR2) > 20 {
		lowR2 = lowR2[:20]
	}

	reviewQueue := buildDebugReviewQueue(storeIssues, issueEmbeddings, tagNames, tagEmbeddings, decomp)

	return DebugFactorWeightsResult{
		FactorWeight:    math.Round(decomp.FactorWeight*1000) / 1000,
		ResidualWeight:  math.Round(decomp.ResidualWeight*1000) / 1000,
		AggregateR2:     math.Round(decomp.AggregateR2*1000) / 1000,
		IssueCount:      len(storeIssues),
		DecomposedCount: decomp.DecomposedCount(),
		Decomposed:      decomp.Decomposed(),
		LowR2Issues:     lowR2,
		ReviewQueue:     reviewQueue,
	}, nil
}

func truncateRaw(s string) string {
	if len(s) <= debugIssueRawPreviewMaxLen {
		return s
	}
	return s[:debugIssueRawPreviewMaxLen] + "..."
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
	ExplainedNorm       float64                 `json:"explainedNorm"`
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
	Catalog *tags.CatalogService
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

	tagNames, tagEmbeddings := tagDataFromIssues(allIssues, storeTags)
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
	decomp := issuemath.ComputeFactorDecomposition(allIssues, tagNames, issueEmbeddings, tagEmbeddings)
	if !decomp.Decomposed() {
		result.Skipped = true
		result.SkipReason = "factor decomposition fell back to static weights"
		return result, nil
	}

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
	if factorNorm, ok := decomp.FactorNorm(target.ID); ok {
		result.ExplainedNorm = math.Round(factorNorm*1000) / 1000
	}
	if factorEmb := decomp.FactorEmbedding(target.ID); len(factorEmb) > 0 {
		result.FactorAlignment = math.Round(vectors.CosineSimilarity(targetEmb, factorEmb)*1000) / 1000
	}
	if residualNorm, ok := decomp.ResidualNorm(target.ID); ok {
		result.ResidualNorm = math.Round(residualNorm*1000) / 1000
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
				Raw:        truncateRaw(issue.Raw),
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

func buildDebugReviewQueue(
	allIssues []issues.Issue,
	issueEmbeddings map[string][]float64,
	tagNames []string,
	tagEmbeddings map[string][]float64,
	decomp issuemath.FactorDecomposition,
) DebugReviewQueue {
	if !decomp.Decomposed() {
		return DebugReviewQueue{}
	}

	embDim := 0
	for _, emb := range tagEmbeddings {
		if len(emb) > 0 {
			embDim = len(emb)
			break
		}
	}
	if embDim == 0 {
		return DebugReviewQueue{}
	}

	issueByID := make(map[string]issues.Issue, len(allIssues))
	for _, issue := range allIssues {
		issueByID[issue.ID] = issue
	}

	var lowR2Issues []DebugReviewIssue
	var lowAlignment []DebugReviewLowAlignment
	var residualMisses []DebugReviewResidualMiss

	for _, issue := range allIssues {
		if issue.Status != issues.StatusOpen {
			continue
		}

		targetEmb := issueEmbeddings[issue.ID]
		if len(targetEmb) != embDim {
			continue
		}
		r2, ok := decomp.IssueR2(issue.ID)
		if !ok {
			continue
		}
		roundedR2 := math.Round(r2*1000) / 1000

		issueTags, nearestResidualTags, residualNeighbors := buildDebugIssueSignals(
			issue,
			targetEmb,
			allIssues,
			issueByID,
			tagNames,
			tagEmbeddings,
			embDim,
			decomp,
		)

		if roundedR2 < 0.15 {
			lowR2Issues = append(lowR2Issues, DebugReviewIssue{
				ID:        issue.ID,
				Raw:       truncateRaw(issue.Raw),
				Status:    issue.Status,
				R2:        roundedR2,
				TagScores: copyDebugTagScores(issue.TagScores),
				Diagnosis: diagnoseR2(DebugIssueR2Result{
					ID:                  issue.ID,
					Raw:                 issue.Raw,
					R2:                  roundedR2,
					TagCount:            len(issueTags),
					Tags:                issueTags,
					NearestResidualTags: nearestResidualTags,
					ResidualNeighbors:   residualNeighbors,
				}),
			})
		}

		for i, tag := range issueTags {
			if tag.Relevance >= 0.3 && tag.Alignment < 0.1 && i < len(issue.TagScores) {
				lowAlignment = append(lowAlignment, DebugReviewLowAlignment{
					IssueID:     issue.ID,
					IssueRaw:    truncateRaw(issue.Raw),
					IssueStatus: issue.Status,
					IssueR2:     roundedR2,
					TagScore:    issue.TagScores[i],
					Alignment:   tag.Alignment,
				})
			}
		}

		unassignedCandidates := make([]DebugResidualTagMatch, 0, len(nearestResidualTags))
		for _, candidate := range nearestResidualTags {
			if candidate.Assigned || candidate.Similarity <= 0.2 {
				continue
			}
			unassignedCandidates = append(unassignedCandidates, candidate)
		}
		if len(unassignedCandidates) > 0 {
			if len(unassignedCandidates) > 3 {
				unassignedCandidates = unassignedCandidates[:3]
			}
			residualMisses = append(residualMisses, DebugReviewResidualMiss{
				IssueID:       issue.ID,
				IssueRaw:      truncateRaw(issue.Raw),
				IssueStatus:   issue.Status,
				IssueR2:       roundedR2,
				TagScores:     copyDebugTagScores(issue.TagScores),
				CandidateTags: unassignedCandidates,
			})
		}
	}

	slices.SortFunc(lowR2Issues, func(a, b DebugReviewIssue) int {
		if c := cmp.Compare(a.R2, b.R2); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	slices.SortFunc(lowAlignment, func(a, b DebugReviewLowAlignment) int {
		if c := cmp.Compare(a.Alignment, b.Alignment); c != 0 {
			return c
		}
		if c := cmp.Compare(b.TagScore.Relevance, a.TagScore.Relevance); c != 0 {
			return c
		}
		return cmp.Compare(a.IssueID, b.IssueID)
	})
	slices.SortFunc(residualMisses, func(a, b DebugReviewResidualMiss) int {
		topA := 0.0
		if len(a.CandidateTags) > 0 {
			topA = a.CandidateTags[0].Similarity
		}
		topB := 0.0
		if len(b.CandidateTags) > 0 {
			topB = b.CandidateTags[0].Similarity
		}
		if c := cmp.Compare(topB, topA); c != 0 {
			return c
		}
		if c := cmp.Compare(a.IssueR2, b.IssueR2); c != 0 {
			return c
		}
		return cmp.Compare(a.IssueID, b.IssueID)
	})

	if len(lowR2Issues) > 12 {
		lowR2Issues = lowR2Issues[:12]
	}
	if len(lowAlignment) > 12 {
		lowAlignment = lowAlignment[:12]
	}
	if len(residualMisses) > 12 {
		residualMisses = residualMisses[:12]
	}

	return DebugReviewQueue{
		LowR2Issues:      lowR2Issues,
		LowAlignmentTags: lowAlignment,
		ResidualMisses:   residualMisses,
	}
}

func buildDebugIssueSignals(
	issue issues.Issue,
	targetEmb []float64,
	allIssues []issues.Issue,
	issueByID map[string]issues.Issue,
	tagNames []string,
	tagEmbeddings map[string][]float64,
	embDim int,
	decomp issuemath.FactorDecomposition,
) ([]DebugIssueR2Tag, []DebugResidualTagMatch, []DebugResidualNeighbor) {
	issueTags := make([]DebugIssueR2Tag, 0, len(issue.TagScores))
	assignedTags := make(map[string]struct{}, len(issue.TagScores))
	for _, ts := range issue.TagScores {
		alignment := 0.0
		if tagEmb, ok := tagEmbeddings[ts.Tag]; ok && len(tagEmb) == embDim {
			alignment = vectors.CosineSimilarity(targetEmb, tagEmb)
		}
		issueTags = append(issueTags, DebugIssueR2Tag{
			Tag:       ts.Tag,
			Relevance: ts.Relevance,
			Alignment: math.Round(alignment*1000) / 1000,
		})
		assignedTags[ts.Tag] = struct{}{}
	}

	nearestResidualTags := make([]DebugResidualTagMatch, 0)
	residualNeighbors := make([]DebugResidualNeighbor, 0)

	residualEmb := decomp.ResidualEmbedding(issue.ID)
	if len(residualEmb) == 0 {
		return issueTags, nearestResidualTags, residualNeighbors
	}

	type tagSim struct {
		tag      string
		sim      float64
		assigned bool
	}
	candidates := make([]tagSim, 0, len(tagNames))
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
		nearestResidualTags = append(nearestResidualTags, DebugResidualTagMatch{
			Tag:        c.tag,
			Similarity: math.Round(c.sim*1000) / 1000,
			Assigned:   c.assigned,
		})
	}

	type neighborSim struct {
		id  string
		sim float64
	}
	neighbors := make([]neighborSim, 0)
	for _, other := range allIssues {
		if other.ID == issue.ID {
			continue
		}
		otherResidual := decomp.ResidualEmbedding(other.ID)
		if len(otherResidual) == 0 {
			continue
		}
		sim := vectors.CosineSimilarity(residualEmb, otherResidual)
		if sim > 0.3 {
			neighbors = append(neighbors, neighborSim{id: other.ID, sim: sim})
		}
	}
	slices.SortFunc(neighbors, func(a, b neighborSim) int {
		return cmp.Compare(b.sim, a.sim)
	})
	neighborLimit := min(8, len(neighbors))
	for _, n := range neighbors[:neighborLimit] {
		other := issueByID[n.id]
		tags := make([]string, 0, len(other.TagScores))
		for _, ts := range other.TagScores {
			tags = append(tags, ts.Tag)
		}
		neighborR2, _ := decomp.IssueR2(n.id)
		residualNeighbors = append(residualNeighbors, DebugResidualNeighbor{
			ID:         n.id,
			Raw:        truncateRaw(other.Raw),
			Similarity: math.Round(n.sim*1000) / 1000,
			R2:         math.Round(neighborR2*1000) / 1000,
			Tags:       tags,
		})
	}

	return issueTags, nearestResidualTags, residualNeighbors
}

func copyDebugTagScores(scores []issues.TagRelevance) []issues.TagRelevance {
	if len(scores) == 0 {
		return nil
	}
	out := make([]issues.TagRelevance, len(scores))
	copy(out, scores)
	return out
}

func tagDataFromIssues(items []issues.Issue, storeTags []issues.Tag) ([]string, map[string][]float64) {
	seen := make(map[string]struct{})
	tagNames := make([]string, 0, len(storeTags))

	for _, tag := range storeTags {
		name := strings.TrimSpace(tag.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		tagNames = append(tagNames, name)
	}
	for _, issue := range items {
		for _, ts := range issue.TagScores {
			name := strings.TrimSpace(ts.Tag)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			tagNames = append(tagNames, name)
		}
	}
	slices.Sort(tagNames)

	tagEmbeddings := make(map[string][]float64, len(storeTags))
	for _, tag := range storeTags {
		name := strings.TrimSpace(tag.Name)
		if name != "" && len(tag.Embedding) > 0 {
			tagEmbeddings[name] = tag.Embedding
		}
	}

	return tagNames, tagEmbeddings
}
