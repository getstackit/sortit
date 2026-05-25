package enrichment

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"sortit/internal/ai"
	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/tags"
)

func TestNormalizeWithOffsetsCollapsesWhitespaceAndCase(t *testing.T) {
	nm := normalizeWithOffsets("  Foo\tBAR\n  baz  ")
	want := "foo bar baz"
	if nm.text != want {
		t.Fatalf("expected %q, got %q", want, nm.text)
	}
}

func TestNormalizeWithOffsetsHandlesSmartPunctuation(t *testing.T) {
	nm := normalizeWithOffsets("\u201CCan\u2019t open\u201D \u2014 export")
	want := "\"can't open\" - export"
	if nm.text != want {
		t.Fatalf("expected %q, got %q", want, nm.text)
	}
}

func TestResolveEvidenceRangesFindsVerbatimQuotes(t *testing.T) {
	raw := "Search box clears after typing the second character."
	ranges := resolveEvidenceRanges(raw, []string{
		"Search box clears",
		"typing the second character",
		"completely fabricated phrase",
		"  ",
	})
	if len(ranges) != 2 {
		t.Fatalf("expected 2 resolved ranges, got %d: %#v", len(ranges), ranges)
	}
	if got := raw[ranges[0].Start:ranges[0].End]; got != "Search box clears" {
		t.Fatalf("first range expected %q, got %q", "Search box clears", got)
	}
	if ranges[0].Text != "Search box clears" || ranges[0].Kind != "direct_quote" || ranges[0].Source != "source_text" {
		t.Fatalf("first range missing citation metadata: %#v", ranges[0])
	}
	if got := raw[ranges[1].Start:ranges[1].End]; got != "typing the second character" {
		t.Fatalf("second range expected %q, got %q", "typing the second character", got)
	}
}

func TestResolveEvidenceRangesIsCaseInsensitive(t *testing.T) {
	raw := "Safari Export Crashes on iPad"
	ranges := resolveEvidenceRanges(raw, []string{"safari export crashes"})
	if len(ranges) != 1 {
		t.Fatalf("expected 1 range, got %d", len(ranges))
	}
	if got := raw[ranges[0].Start:ranges[0].End]; got != "Safari Export Crashes" {
		t.Fatalf("expected %q, got %q", "Safari Export Crashes", got)
	}
}

func TestResolveEvidenceRangesReturnsNilForEmptyRaw(t *testing.T) {
	if got := resolveEvidenceRanges("", []string{"anything"}); got != nil {
		t.Fatalf("expected nil for empty raw, got %#v", got)
	}
}

func TestResolveEvidenceRangesReturnsNilForNoMatch(t *testing.T) {
	if got := resolveEvidenceRanges("hello world", []string{"not here"}); got != nil {
		t.Fatalf("expected nil for unmatched quotes, got %#v", got)
	}
}

type evidenceTagger struct {
	scores []ai.TagScore
}

func (t *evidenceTagger) Score(_ context.Context, _ string, _ []ai.Tag, _ []ai.FewShotExample) (ai.ScoreResult, error) {
	return ai.ScoreResult{Tags: append([]ai.TagScore(nil), t.scores...)}, nil
}

func (t *evidenceTagger) Provider() string { return "test" }
func (t *evidenceTagger) Model() string    { return "test" }

func newEvidenceEnricher(t *testing.T, tagger ai.Tagger, storeTags []issues.Tag) *IssueEnricher {
	t.Helper()
	store := &catalogTestStore{tags: storeTags}
	embedder := &enricherTestEmbedder{
		vectors: map[string][]float32{
			"raw text with search box clears": {1, 0},
		},
	}
	analyzer := ai.NewAnalyzer(tagger, embedder)
	catalog := tags.NewCatalogService(store, analyzer, slog.Default())
	enricher := NewIssueEnricher(analyzer, catalog, slog.Default())
	enricher.SetExemplarPool(nil)
	return enricher
}

func findScore(t *testing.T, scores []issues.TagRelevance, name string) issues.TagRelevance {
	t.Helper()
	for _, score := range scores {
		if score.Tag == name {
			return score
		}
	}
	t.Fatalf("tag %q not found in %#v", name, scores)
	return issues.TagRelevance{}
}

func TestAnalyzeTextKeepsTagWithMatchedEvidenceRanges(t *testing.T) {
	tagger := &evidenceTagger{
		scores: []ai.TagScore{{
			Tag:       "database",
			Relevance: 0.9,
			Evidence:  []string{"raw text"},
		}},
	}
	enricher := newEvidenceEnricher(t, tagger, []issues.Tag{
		{Name: "database", Embedding: []float64{1, 0}},
	})

	result, err := enricher.AnalyzeText(context.Background(), "raw text with search box clears", AnalyzeTextOptions{
		CandidateMode: tags.CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}

	score := findScore(t, result.TagScores, "database")
	if score.VerificationVerdict != domain.TagVerificationVerdictKeep {
		t.Fatalf("expected keep verdict for matched evidence, got %#v", score)
	}
	if len(score.Evidence) != 1 {
		t.Fatalf("expected 1 evidence range, got %d", len(score.Evidence))
	}
	if score.Evidence[0].Start != 0 || score.Evidence[0].End != 8 {
		t.Fatalf("expected evidence range [0,8), got %#v", score.Evidence[0])
	}
	if !strings.Contains(score.VerificationReason, "evidence") {
		t.Fatalf("expected reason to mention evidence, got %q", score.VerificationReason)
	}
}

func TestAnalyzeTextKeepsTagWithoutEvidenceNoDownrank(t *testing.T) {
	tagger := &evidenceTagger{
		scores: []ai.TagScore{{
			Tag:       "database",
			Relevance: 0.9,
		}},
	}
	enricher := newEvidenceEnricher(t, tagger, []issues.Tag{
		{Name: "database", Embedding: []float64{1, 0}},
	})

	result, err := enricher.AnalyzeText(context.Background(), "raw text with search box clears", AnalyzeTextOptions{
		CandidateMode: tags.CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}

	score := findScore(t, result.TagScores, "database")
	if score.VerificationVerdict != domain.TagVerificationVerdictKeep {
		t.Fatalf("expected keep for tag without evidence (no penalty), got %#v", score)
	}
	if len(score.Evidence) != 0 {
		t.Fatalf("expected no evidence ranges when no quotes provided, got %#v", score.Evidence)
	}
}

func TestAnalyzeTextEvidenceRescuesWeakAlignment(t *testing.T) {
	tagger := &evidenceTagger{
		scores: []ai.TagScore{{
			Tag:       "cleanup",
			Relevance: 0.9,
			Evidence:  []string{"search box clears"},
		}},
	}
	cleanupSpecificity := 0.2
	databaseSpecificity := 0.8
	enricher := newEvidenceEnricher(t, tagger, []issues.Tag{
		{Name: "cleanup", Embedding: []float64{0, 1}, Specificity: &cleanupSpecificity},
		{Name: "database", Embedding: []float64{1, 0}, Specificity: &databaseSpecificity},
	})

	result, err := enricher.AnalyzeText(context.Background(), "raw text with search box clears", AnalyzeTextOptions{
		CandidateMode: tags.CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}

	score := findScore(t, result.TagScores, "cleanup")
	if score.VerificationVerdict != domain.TagVerificationVerdictKeep {
		t.Fatalf("expected evidence to rescue weak alignment, got %#v", score)
	}
	if !strings.Contains(score.VerificationReason, "rescued") {
		t.Fatalf("expected reason to mention rescue, got %q", score.VerificationReason)
	}
}

func TestIssueTagScoresFromAnalysisDoesNotCopyEvidence(t *testing.T) {
	scores := IssueTagScoresFromAnalysis([]ai.TagScore{
		{Tag: "bug", Relevance: 0.8, Evidence: []string{"something broke"}},
	})
	if len(scores) != 1 {
		t.Fatalf("expected 1 score, got %d", len(scores))
	}
	if len(scores[0].Evidence) != 0 {
		t.Fatalf("expected no evidence on TagRelevance (resolved later), got %#v", scores[0].Evidence)
	}
}
