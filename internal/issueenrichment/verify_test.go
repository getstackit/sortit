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

func TestNormalizeForEvidenceMatchCollapsesWhitespaceAndCase(t *testing.T) {
	got := normalizeForEvidenceMatch("  Foo\tBAR\n  baz  ")
	want := "foo bar baz"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNormalizeForEvidenceMatchNormalizesSmartPunctuation(t *testing.T) {
	got := normalizeForEvidenceMatch("\u201CCan\u2019t open\u201D \u2014 export")
	want := "\"can't open\" - export"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestCountMatchedEvidenceCountsOnlyVerbatimQuotes(t *testing.T) {
	raw := normalizeForEvidenceMatch("Search box clears after typing the second character on the open issues page.")
	matched := countMatchedEvidence([]string{
		"Search box clears",                  // verbatim
		"clears after typing the SECOND",     // verbatim modulo case
		"completely fabricated phrase",       // hallucinated
		"  ",                                 // empty after trim
	}, raw)
	if matched != 2 {
		t.Fatalf("expected 2 matches, got %d", matched)
	}
}

func TestCountMatchedEvidenceReturnsZeroForEmptyRaw(t *testing.T) {
	if got := countMatchedEvidence([]string{"anything"}, ""); got != 0 {
		t.Fatalf("expected 0 matches for empty raw, got %d", got)
	}
}

type evidenceTagger struct {
	scores []ai.TagScore
}

func (t *evidenceTagger) Score(_ context.Context, _ string, _ []ai.Tag, _ []ai.FewShotExample) ([]ai.TagScore, error) {
	return append([]ai.TagScore(nil), t.scores...), nil
}

func (t *evidenceTagger) Provider() string { return "test" }
func (t *evidenceTagger) Model() string    { return "test" }

func newEvidenceEnricher(t *testing.T, tagger ai.Tagger, storeTags []issues.Tag) *IssueEnricher {
	t.Helper()
	store := &catalogTestStore{tags: storeTags}
	embedder := &enricherTestEmbedder{
		vectors: map[string][]float32{
			"raw text": {1, 0},
		},
	}
	analyzer := ai.NewAnalyzer(tagger, embedder)
	catalog := tags.NewCatalogService(store, analyzer, slog.Default())
	enricher := NewIssueEnricher(analyzer, catalog, slog.Default())
	enricher.SetExemplarPool(nil)
	return enricher
}

// findScore returns the score for the named tag or t.Fatals if not found.
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

func TestAnalyzeTextDownranksHighRelevanceTagWithUnmatchedEvidence(t *testing.T) {
	tagger := &evidenceTagger{
		scores: []ai.TagScore{{
			Tag:       "database",
			Relevance: 0.9,
			Evidence:  []string{"hallucinated phrase that is not in the source"},
		}},
	}
	enricher := newEvidenceEnricher(t, tagger, []issues.Tag{
		{Name: "database", Embedding: []float64{1, 0}},
	})

	result, err := enricher.AnalyzeText(context.Background(), "raw text", AnalyzeTextOptions{
		CandidateMode: tags.CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}

	score := findScore(t, result.TagScores, "database")
	if score.VerificationVerdict != domain.TagVerificationVerdictFlagged {
		t.Fatalf("expected flagged verdict for unmatched evidence at high relevance, got %#v", score)
	}
	if score.EvidenceMatched == nil || *score.EvidenceMatched != 0 {
		t.Fatalf("expected EvidenceMatched=0, got %#v", score.EvidenceMatched)
	}
	if !strings.Contains(score.VerificationReason, "evidence") {
		t.Fatalf("expected verification reason to mention evidence, got %q", score.VerificationReason)
	}
}

func TestAnalyzeTextKeepsTagWithMatchedEvidence(t *testing.T) {
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

	result, err := enricher.AnalyzeText(context.Background(), "raw text", AnalyzeTextOptions{
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
	if score.EvidenceMatched == nil || *score.EvidenceMatched != 1 {
		t.Fatalf("expected EvidenceMatched=1, got %#v", score.EvidenceMatched)
	}
}

func TestAnalyzeTextFlagsSuggestedTagWithoutEvidence(t *testing.T) {
	tagger := &evidenceTagger{
		scores: []ai.TagScore{{
			Tag:         "novel-tag",
			Relevance:   0.5,
			Suggested:   true,
			Description: "made-up new concept",
		}},
	}
	enricher := newEvidenceEnricher(t, tagger, nil)

	result, err := enricher.AnalyzeText(context.Background(), "raw text", AnalyzeTextOptions{
		CandidateMode: tags.CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}

	score := findScore(t, result.TagScores, "novel-tag")
	if score.VerificationVerdict != domain.TagVerificationVerdictFlagged {
		t.Fatalf("expected flagged verdict for suggested tag without evidence, got %#v", score)
	}
	if !strings.Contains(score.VerificationReason, "suggested tag") {
		t.Fatalf("expected verification reason to mention suggested tag, got %q", score.VerificationReason)
	}
}

func TestAnalyzeTextLeavesLowRelevanceUnverifiedEvidenceAlone(t *testing.T) {
	tagger := &evidenceTagger{
		scores: []ai.TagScore{{
			Tag:       "database",
			Relevance: 0.2,
			Evidence:  []string{"not present in source"},
		}},
	}
	enricher := newEvidenceEnricher(t, tagger, []issues.Tag{
		{Name: "database", Embedding: []float64{1, 0}},
	})

	result, err := enricher.AnalyzeText(context.Background(), "raw text", AnalyzeTextOptions{
		CandidateMode: tags.CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}

	score := findScore(t, result.TagScores, "database")
	if score.VerificationVerdict != domain.TagVerificationVerdictKeep {
		t.Fatalf("expected keep verdict for low-relevance unmatched evidence, got %#v", score)
	}
}

func TestIssueTagScoresFromAnalysisCarriesEvidenceQuotes(t *testing.T) {
	scores := IssueTagScoresFromAnalysis([]ai.TagScore{
		{Tag: "bug", Relevance: 0.8, Evidence: []string{"  something broke  ", "", "another quote"}},
	})

	if len(scores) != 1 {
		t.Fatalf("expected 1 score, got %d", len(scores))
	}
	if got, want := scores[0].Evidence, []string{"something broke", "another quote"}; !equalStringSlices(got, want) {
		t.Fatalf("expected trimmed evidence %#v, got %#v", want, got)
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
