package diagnostics

import (
	"context"
	"log/slog"
	"math"
	"testing"
	"time"

	"sortit/internal/ai"
	issueenrichment "sortit/internal/issueenrichment"
	"sortit/internal/issues"
	"sortit/internal/tags"
)

const testFakeProvider = "fake"
const testBugTag = "bug"

type debugTagStore struct {
	tags []issues.Tag
}

func (s *debugTagStore) ListTags(context.Context) ([]issues.Tag, error) {
	out := make([]issues.Tag, len(s.tags))
	copy(out, s.tags)
	return out, nil
}

func (s *debugTagStore) UpsertTags(_ context.Context, tags []issues.Tag) error {
	out := make([]issues.Tag, len(tags))
	copy(out, tags)
	s.tags = out
	return nil
}

func (s *debugTagStore) UpdateTagSpecificity(context.Context, string, *float64, *float64, *float64, *time.Time) error {
	return nil
}

func TestDebugFactorWeightsReportsFallbackState(t *testing.T) {
	ctx := context.Background()
	store := issues.NewInMemoryStore([]issues.Issue{
		{ID: "issue-a", Raw: "a", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{ID: "issue-b", Raw: "b", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{ID: "issue-c", Raw: "c", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{ID: "issue-d", Raw: "d", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{ID: "issue-e", Raw: "e", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}},
		{ID: "issue-f", Raw: "f", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}},
	})
	tagStore := &debugTagStore{}
	catalog := tags.NewCatalogService(tagStore, nil, nil)
	if err := catalog.EnsureStoredTags(ctx, []issues.Tag{
		{Name: "alpha", Embedding: unitVec64([]float64{1, 0, 0, 0})},
	}); err != nil {
		t.Fatalf("ensure stored tags: %v", err)
	}

	result, err := (DebugFactorWeightsHandler{Store: store, Catalog: catalog}).Handle(ctx)
	if err != nil {
		t.Fatalf("handle debug factor weights: %v", err)
	}
	if result.Decomposed {
		t.Fatal("expected factor decomposition to report fallback state")
	}
	if result.DecomposedCount != 0 {
		t.Fatalf("expected 0 decomposed issues, got %d", result.DecomposedCount)
	}
}

func TestDebugFactorWeightsIncludesLowR2IssueStatus(t *testing.T) {
	ctx := context.Background()
	store := issues.NewInMemoryStore([]issues.Issue{
		{ID: "issue-a", Raw: "a", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{ID: "issue-b", Raw: "b", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{ID: "issue-c", Raw: "c", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{ID: "issue-d", Raw: "d", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{ID: "issue-open-low", Raw: "open low", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{0, 1, 0, 0})},
		{ID: "issue-closed-low", Raw: "closed low", Status: issues.StatusClosed, TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{0, 1, 0, 0})},
	})
	tagStore := &debugTagStore{}
	catalog := tags.NewCatalogService(tagStore, nil, nil)
	if err := catalog.EnsureStoredTags(ctx, []issues.Tag{
		{Name: "alpha", Embedding: unitVec64([]float64{1, 0, 0, 0})},
	}); err != nil {
		t.Fatalf("ensure stored tags: %v", err)
	}

	result, err := (DebugFactorWeightsHandler{Store: store, Catalog: catalog}).Handle(ctx)
	if err != nil {
		t.Fatalf("handle debug factor weights: %v", err)
	}

	statusByID := make(map[string]issues.IssueStatus, len(result.LowR2Issues))
	for _, item := range result.LowR2Issues {
		statusByID[item.ID] = item.Status
	}
	if statusByID["issue-open-low"] != issues.StatusOpen {
		t.Fatalf("issue-open-low status = %q, want %q", statusByID["issue-open-low"], issues.StatusOpen)
	}
	if statusByID["issue-closed-low"] != issues.StatusClosed {
		t.Fatalf("issue-closed-low status = %q, want %q", statusByID["issue-closed-low"], issues.StatusClosed)
	}
}

func TestDebugFactorWeightsBuildsActionableReviewQueue(t *testing.T) {
	const openReviewIssueID = "issue-open-review"

	ctx := context.Background()
	store := issues.NewInMemoryStore([]issues.Issue{
		{ID: "issue-a", Raw: "a", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{ID: "issue-b", Raw: "b", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{ID: "issue-c", Raw: "c", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{ID: "issue-d", Raw: "d", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{
			ID:     openReviewIssueID,
			Raw:    "beta concept hidden behind alpha tag",
			Status: issues.StatusOpen,
			TagScores: []issues.TagRelevance{
				{Tag: "alpha", Relevance: 0.91, Suggested: true, Description: "legacy broad alpha bucket"},
			},
			Embedding: unitVec64([]float64{0, 1, 0, 0}),
		},
		{
			ID:        "issue-closed-review",
			Raw:       "closed review case",
			Status:    issues.StatusClosed,
			TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 0.9}},
			Embedding: unitVec64([]float64{0, 1, 0, 0}),
		},
	})
	tagStore := &debugTagStore{}
	catalog := tags.NewCatalogService(tagStore, nil, nil)
	if err := catalog.EnsureStoredTags(ctx, []issues.Tag{
		{Name: "alpha", Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{Name: "beta", Embedding: unitVec64([]float64{0, 1, 0, 0})},
	}); err != nil {
		t.Fatalf("ensure stored tags: %v", err)
	}

	result, err := (DebugFactorWeightsHandler{Store: store, Catalog: catalog}).Handle(ctx)
	if err != nil {
		t.Fatalf("handle debug factor weights: %v", err)
	}

	if len(result.ReviewQueue.LowR2Issues) == 0 {
		t.Fatal("expected low-R2 review issues")
	}
	lowR2 := result.ReviewQueue.LowR2Issues[0]
	if lowR2.ID != openReviewIssueID {
		t.Fatalf("expected %s first in low-R2 queue, got %#v", openReviewIssueID, result.ReviewQueue.LowR2Issues)
	}
	if len(lowR2.TagScores) != 1 || !lowR2.TagScores[0].Suggested {
		t.Fatalf("expected suggested tag provenance in low-R2 queue, got %#v", lowR2.TagScores)
	}
	if lowR2.TagScores[0].Description != "legacy broad alpha bucket" {
		t.Fatalf("unexpected low-R2 description %q", lowR2.TagScores[0].Description)
	}
	if len(lowR2.Diagnosis) == 0 {
		t.Fatal("expected low-R2 diagnosis")
	}

	if len(result.ReviewQueue.LowAlignmentTags) == 0 {
		t.Fatal("expected low-alignment review tags")
	}
	lowAlignment := result.ReviewQueue.LowAlignmentTags[0]
	if lowAlignment.IssueID != openReviewIssueID {
		t.Fatalf("expected %s low-alignment entry, got %#v", openReviewIssueID, result.ReviewQueue.LowAlignmentTags)
	}
	if lowAlignment.TagScore.Tag != "alpha" || !lowAlignment.TagScore.Suggested {
		t.Fatalf("unexpected low-alignment tag score %#v", lowAlignment.TagScore)
	}
	if lowAlignment.Alignment >= 0.1 {
		t.Fatalf("expected suspicious alignment < 0.1, got %v", lowAlignment.Alignment)
	}

	if len(result.ReviewQueue.ResidualMisses) == 0 {
		t.Fatal("expected residual-miss review entries")
	}
	residualMiss := result.ReviewQueue.ResidualMisses[0]
	if residualMiss.IssueID != openReviewIssueID {
		t.Fatalf("expected %s residual miss, got %#v", openReviewIssueID, result.ReviewQueue.ResidualMisses)
	}
	if len(residualMiss.CandidateTags) == 0 || residualMiss.CandidateTags[0].Tag != "beta" {
		t.Fatalf("expected beta residual candidate, got %#v", residualMiss.CandidateTags)
	}
	for _, item := range result.ReviewQueue.LowR2Issues {
		if item.ID == "issue-closed-review" {
			t.Fatal("expected closed issues to be excluded from actionable low-R2 review queue")
		}
	}
}

func TestDebugIssueR2ReportsRawNorms(t *testing.T) {
	ctx := context.Background()
	store := issues.NewInMemoryStore([]issues.Issue{
		{ID: "pure-a", Raw: "test", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{ID: "mixed-a", Raw: "test", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 1, 0})},
		{ID: "mixed-b", Raw: "test", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 1, 0})},
		{ID: "mixed-c", Raw: "test", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 1, 0})},
		{ID: "mixed-d", Raw: "test", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 1, 0})},
	})
	tagStore := &debugTagStore{}
	catalog := tags.NewCatalogService(tagStore, nil, nil)
	if err := catalog.EnsureStoredTags(ctx, []issues.Tag{
		{Name: "alpha", Embedding: unitVec64([]float64{1, 0, 0, 0})},
	}); err != nil {
		t.Fatalf("ensure stored tags: %v", err)
	}

	result, err := (DebugIssueR2Handler{Store: store, Catalog: catalog}).Handle(ctx, "mixed-a")
	if err != nil {
		t.Fatalf("handle debug issue r2: %v", err)
	}
	if result.Skipped {
		t.Fatalf("expected decomposition-backed diagnosis, got skipped=%v reason=%q", result.Skipped, result.SkipReason)
	}

	want := math.Round(math.Sqrt(0.5)*1000) / 1000
	if result.ExplainedNorm != want {
		t.Fatalf("expected explained norm %v, got %v", want, result.ExplainedNorm)
	}
	if result.ResidualNorm != want {
		t.Fatalf("expected residual norm %v, got %v", want, result.ResidualNorm)
	}
}

type debugEvalTagger struct {
	scoresByText map[string][]ai.TagScore
}

func (t *debugEvalTagger) Score(_ context.Context, text string, _ []ai.Tag, _ []ai.FewShotExample) (ai.ScoreResult, error) {
	return ai.ScoreResult{Tags: append([]ai.TagScore(nil), t.scoresByText[text]...)}, nil
}

func (t *debugEvalTagger) Provider() string {
	return testFakeProvider
}

func (t *debugEvalTagger) Model() string {
	return "eval-tagger"
}

type debugEvalSequenceTagger struct {
	sequences map[string][][]ai.TagScore
	calls     map[string]int
}

func (t *debugEvalSequenceTagger) Score(_ context.Context, text string, _ []ai.Tag, _ []ai.FewShotExample) (ai.ScoreResult, error) {
	if t.calls == nil {
		t.calls = make(map[string]int)
	}
	sequence := t.sequences[text]
	if len(sequence) == 0 {
		return ai.ScoreResult{}, nil
	}
	index := t.calls[text]
	if index >= len(sequence) {
		index = len(sequence) - 1
	}
	t.calls[text]++
	return ai.ScoreResult{Tags: append([]ai.TagScore(nil), sequence[index]...)}, nil
}

func (t *debugEvalSequenceTagger) Provider() string {
	return testFakeProvider
}

func (t *debugEvalSequenceTagger) Model() string {
	return "eval-sequence-tagger"
}

type debugEvalEmbedder struct{}

func (e *debugEvalEmbedder) EmbedText(context.Context, string) (ai.EmbeddingResult, error) {
	return ai.EmbeddingResult{}, nil
}

func (e *debugEvalEmbedder) Provider() string {
	return testFakeProvider
}

func (e *debugEvalEmbedder) Model() string {
	return "eval-embedder"
}

func TestDebugEvalTagsReportsAggregateMetrics(t *testing.T) {
	ctx := context.Background()
	analyzer := ai.NewAnalyzer(&debugEvalTagger{
		scoresByText: map[string][]ai.TagScore{
			"issue a": {
				{Tag: "export", Relevance: 0.91},
				{Tag: testBugTag, Relevance: 0.44},
				{Tag: "noise", Relevance: 0.07},
			},
			"issue b": {
				{Tag: "search", Relevance: 0.95},
			},
		},
	}, &debugEvalEmbedder{})
	catalog := tags.NewCatalogService(&debugTagStore{}, analyzer, nil)
	handler := DebugEvalTagsHandler{
		Analyzer: analyzer,
		Catalog:  catalog,
		Enricher: issueenrichment.NewIssueEnricher(analyzer, catalog, slog.Default()),
		Fixture: []DebugTagEvalCase{
			{ID: "a", Text: "issue a", ExpectedTags: []string{"export", "safari"}},
			{ID: "b", Text: "issue b", ExpectedTags: []string{"search"}},
		},
	}

	result, err := handler.Handle(ctx)
	if err != nil {
		t.Fatalf("handle debug eval tags: %v", err)
	}
	if result.CaseCount != 2 {
		t.Fatalf("expected 2 cases, got %d", result.CaseCount)
	}
	if result.Precision != 0.667 {
		t.Fatalf("expected aggregate precision 0.667, got %v", result.Precision)
	}
	if result.Recall != 0.667 {
		t.Fatalf("expected aggregate recall 0.667, got %v", result.Recall)
	}
	if result.ExactMatchCount != 1 {
		t.Fatalf("expected 1 exact match, got %d", result.ExactMatchCount)
	}
	if got := result.Cases[0].MissingTags; len(got) != 1 || got[0] != "safari" {
		t.Fatalf("expected missing safari, got %+v", got)
	}
	if got := result.Cases[0].UnexpectedTags; len(got) != 1 || got[0] != testBugTag {
		t.Fatalf("expected unexpected bug, got %+v", got)
	}
	if got := result.Cases[0].ActualTags; len(got) != 2 || got[0] != "export" || got[1] != testBugTag {
		t.Fatalf("unexpected actual tags %+v", got)
	}
}

func TestDebugEvalTagsLoadsNamedEmbeddedFixtures(t *testing.T) {
	handler := DebugEvalTagsHandler{}

	corpus, corpusName, err := handler.fixtureCases("corpus")
	if err != nil {
		t.Fatalf("fixtureCases(corpus): %v", err)
	}
	if corpusName != "corpus" {
		t.Fatalf("expected corpus fixture name, got %q", corpusName)
	}
	if len(corpus) == 0 {
		t.Fatal("expected corpus fixture to contain cases")
	}
	if corpus[0].SourceIssueID == "" {
		t.Fatal("expected corpus fixture cases to retain source issue ids")
	}

	synthetic, syntheticName, err := handler.fixtureCases("synthetic")
	if err != nil {
		t.Fatalf("fixtureCases(synthetic): %v", err)
	}
	if syntheticName != "synthetic" {
		t.Fatalf("expected synthetic fixture name, got %q", syntheticName)
	}
	if len(synthetic) == 0 {
		t.Fatal("expected synthetic fixture to contain cases")
	}
}

func TestDebugEvalTagsReportsStabilityAcrossRuns(t *testing.T) {
	ctx := context.Background()
	analyzer := ai.NewAnalyzer(&debugEvalSequenceTagger{
		sequences: map[string][][]ai.TagScore{
			"issue a": {
				{{Tag: "export", Relevance: 0.91}},
				{{Tag: "export", Relevance: 0.91}, {Tag: testBugTag, Relevance: 0.44}},
				{{Tag: "export", Relevance: 0.91}},
			},
		},
	}, &debugEvalEmbedder{})
	catalog := tags.NewCatalogService(&debugTagStore{}, analyzer, nil)
	handler := DebugEvalTagsHandler{
		Analyzer: analyzer,
		Catalog:  catalog,
		Enricher: issueenrichment.NewIssueEnricher(analyzer, catalog, slog.Default()),
		Fixture: []DebugTagEvalCase{
			{ID: "a", Text: "issue a", ExpectedTags: []string{"export"}},
		},
	}

	result, err := handler.HandleFixture(ctx, "", 3, tags.CandidateModeRetrievalShortlist, true)
	if err != nil {
		t.Fatalf("HandleFixture runs=3: %v", err)
	}
	if result.Runs != 3 {
		t.Fatalf("expected 3 runs, got %d", result.Runs)
	}
	if result.ExactStableCount != 0 {
		t.Fatalf("expected 0 exactly stable cases, got %d", result.ExactStableCount)
	}
	if result.ExactStability != 0 {
		t.Fatalf("expected exact stability 0, got %v", result.ExactStability)
	}
	if result.AverageStability != 0.75 {
		t.Fatalf("expected average stability 0.75, got %v", result.AverageStability)
	}
	if result.Cases[0].StabilityExact {
		t.Fatal("expected case stabilityExact to be false")
	}
	if result.Cases[0].StabilityJaccard != 0.75 {
		t.Fatalf("expected case stability jaccard 0.75, got %v", result.Cases[0].StabilityJaccard)
	}
	if got := result.Cases[0].AlternateActualTags; len(got) != 1 || len(got[0]) != 2 || got[0][0] != "export" || got[0][1] != testBugTag {
		t.Fatalf("unexpected alternate actual tags %+v", got)
	}
	if !result.VerifierEnabled {
		t.Fatal("expected verifier to default on")
	}
}

func unitVec64(v []float64) []float64 {
	out := make([]float64, len(v))
	copy(out, v)
	var mag float64
	for _, val := range out {
		mag += val * val
	}
	if mag > 0 {
		mag = math.Sqrt(mag)
		for i := range out {
			out[i] /= mag
		}
	}
	return out
}
