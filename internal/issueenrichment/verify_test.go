package enrichment

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"sortit/internal/ai"
	"sortit/internal/domain"
	"sortit/internal/issuemath"
	"sortit/internal/issues"
	"sortit/internal/tags"
)

type fixedCenterer struct{ means issuemath.CorpusMeans }

func (c fixedCenterer) Current(_ context.Context) (issuemath.CorpusMeans, error) {
	return c.means, nil
}

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
	scores  []ai.TagScore
	negated []ai.NegatedTag
}

func (t *evidenceTagger) Score(_ context.Context, _ string, _ []ai.Tag, _ []ai.FewShotExample, _ []ai.PriorDecision, _ ai.ConceptFrame) (ai.ScoreResult, error) {
	return ai.ScoreResult{
		Tags:    append([]ai.TagScore(nil), t.scores...),
		Negated: append([]ai.NegatedTag(nil), t.negated...),
	}, nil
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

func TestAnalyzeTextAppliesAnalyzerNegationWithEvidence(t *testing.T) {
	tagger := &evidenceTagger{
		scores: []ai.TagScore{{Tag: "database", Relevance: 0.9, Evidence: []string{"raw text"}}},
		negated: []ai.NegatedTag{
			{Tag: "cleanup", Confidence: 0.6, Evidence: []string{"search box clears"}},
		},
	}
	enricher := newEvidenceEnricher(t, tagger, []issues.Tag{
		{Name: "database", Embedding: []float64{1, 0}},
		{Name: "cleanup", Embedding: []float64{0, 1}},
	})

	result, err := enricher.AnalyzeText(context.Background(), "raw text with search box clears", AnalyzeTextOptions{
		CandidateMode: tags.CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}

	// "cleanup" was not assigned positively but the negation creates a synthetic row.
	cleanup := findScore(t, result.TagScores, "cleanup")
	if cleanup.Relevance != 0 {
		t.Fatalf("expected synthetic row to have Relevance 0, got %v", cleanup.Relevance)
	}
	if cleanup.Negation == nil || *cleanup.Negation != 0.6 {
		t.Fatalf("expected negation 0.6, got %#v", cleanup.Negation)
	}
	if cleanup.NegationProvenance != domain.NegationProvenanceAnalyzer {
		t.Fatalf("expected analyzer provenance, got %q", cleanup.NegationProvenance)
	}
	if len(cleanup.NegationEvidence) == 0 {
		t.Fatalf("expected negation evidence to be resolved")
	}
	if cleanup.NegationReason == "" {
		t.Fatalf("expected negation reason to be set")
	}
}

func TestAnalyzeTextDiscardsNegationWithoutEvidence(t *testing.T) {
	tagger := &evidenceTagger{
		scores: []ai.TagScore{{Tag: "database", Relevance: 0.9, Evidence: []string{"raw text"}}},
		negated: []ai.NegatedTag{
			// "completely fabricated phrase" never appears in the source text.
			{Tag: "cleanup", Confidence: 0.9, Evidence: []string{"completely fabricated phrase"}},
		},
	}
	enricher := newEvidenceEnricher(t, tagger, []issues.Tag{
		{Name: "database", Embedding: []float64{1, 0}},
		{Name: "cleanup", Embedding: []float64{0, 1}},
	})

	result, err := enricher.AnalyzeText(context.Background(), "raw text with search box clears", AnalyzeTextOptions{
		CandidateMode: tags.CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}

	for _, score := range result.TagScores {
		if score.Tag == "cleanup" {
			t.Fatalf("expected cleanup row to be discarded entirely (no evidence), got %#v", score)
		}
	}
}

func TestAnalyzeTextNegationOnAssignedTagWritesToExistingRow(t *testing.T) {
	tagger := &evidenceTagger{
		scores: []ai.TagScore{
			{Tag: "database", Relevance: 0.9, Evidence: []string{"raw text"}},
			{Tag: "cleanup", Relevance: 0.4},
		},
		negated: []ai.NegatedTag{
			{Tag: "cleanup", Confidence: 0.5, Evidence: []string{"search box clears"}},
		},
	}
	enricher := newEvidenceEnricher(t, tagger, []issues.Tag{
		{Name: "database", Embedding: []float64{1, 0}},
		{Name: "cleanup", Embedding: []float64{0, 1}},
	})

	result, err := enricher.AnalyzeText(context.Background(), "raw text with search box clears", AnalyzeTextOptions{
		CandidateMode: tags.CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}

	cleanup := findScore(t, result.TagScores, "cleanup")
	if cleanup.Relevance == 0 {
		t.Fatalf("expected existing relevance preserved, got 0")
	}
	if cleanup.Negation == nil || *cleanup.Negation != 0.5 {
		t.Fatalf("expected negation 0.5 on existing row, got %#v", cleanup.Negation)
	}
	if cleanup.NegationProvenance != domain.NegationProvenanceAnalyzer {
		t.Fatalf("expected analyzer provenance, got %q", cleanup.NegationProvenance)
	}
}

func TestVerifierDownRankEmitsNegationInsteadOfMutatingRelevance(t *testing.T) {
	// Setup: cleanup gets the AI tag (Relevance 0.3 — under the Flagged
	// threshold of 0.4 so we route to DownRank instead) with embedding {0,1}
	// orthogonal to the issue {1,0} (alignment 0). The unassigned "database"
	// candidate has embedding {1,0} aligning perfectly with the issue
	// (alignment 1.0), triggering the dominance branch.
	tagger := &evidenceTagger{
		scores: []ai.TagScore{{Tag: "cleanup", Relevance: 0.3}},
	}
	enricher := newEvidenceEnricher(t, tagger, []issues.Tag{
		{Name: "cleanup", Embedding: []float64{0, 1}},
		{Name: "database", Embedding: []float64{1, 0}},
	})

	result, err := enricher.AnalyzeText(context.Background(), "raw text with search box clears", AnalyzeTextOptions{
		CandidateMode: tags.CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}

	cleanup := findScore(t, result.TagScores, "cleanup")
	if cleanup.VerificationVerdict != domain.TagVerificationVerdictDownRank {
		t.Fatalf("expected DownRank verdict, got %q", cleanup.VerificationVerdict)
	}
	// Relevance must not be mutated by the down-rank branch anymore.
	if cleanup.Relevance != 0.3 {
		t.Fatalf("expected Relevance preserved at 0.3, got %v", cleanup.Relevance)
	}
	if cleanup.Negation == nil {
		t.Fatalf("expected Negation to be set on DownRank verdict, got nil")
	}
	if *cleanup.Negation != verifierDominanceNegation {
		t.Fatalf("expected Negation %v, got %v", verifierDominanceNegation, *cleanup.Negation)
	}
	if cleanup.NegationProvenance != domain.NegationProvenanceVerifier {
		t.Fatalf("expected verifier-dominance provenance, got %q", cleanup.NegationProvenance)
	}
	if cleanup.NegationReason == "" {
		t.Fatalf("expected NegationReason to be set")
	}
}

func TestVerifierSuppressesAntiAlignedTag(t *testing.T) {
	// backend is assigned by the AI with no grounded evidence, but its embedding
	// {-1,0} points opposite the issue embedding {1,0} (cosine -1). It is the
	// generic-tag drag: suppressed via a strong negation, Relevance preserved.
	tagger := &evidenceTagger{
		scores: []ai.TagScore{{Tag: "backend", Relevance: 0.5}},
	}
	enricher := newEvidenceEnricher(t, tagger, []issues.Tag{
		{Name: "backend", Embedding: []float64{-1, 0}},
	})

	result, err := enricher.AnalyzeText(context.Background(), "raw text with search box clears", AnalyzeTextOptions{
		CandidateMode: tags.CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}

	backend := findScore(t, result.TagScores, "backend")
	if backend.VerificationVerdict != domain.TagVerificationVerdictDownRank {
		t.Fatalf("expected DownRank for anti-aligned tag, got %q", backend.VerificationVerdict)
	}
	if backend.Relevance != 0.5 {
		t.Fatalf("expected Relevance preserved at 0.5, got %v", backend.Relevance)
	}
	if backend.Negation == nil || *backend.Negation != verifierAntiAlignmentNegation {
		t.Fatalf("expected anti-alignment negation %v, got %#v", verifierAntiAlignmentNegation, backend.Negation)
	}
	if backend.NegationProvenance != domain.NegationProvenanceVerifier {
		t.Fatalf("expected verifier provenance, got %q", backend.NegationProvenance)
	}
}

func TestVerifierSuppressesAntiAlignedTagEvenWithEvidence(t *testing.T) {
	// Same anti-aligned embedding, but now the analyzer attaches source-text
	// evidence. This is the generic-mention case ("backend" appears in the text but
	// the issue points elsewhere): anti-alignment must override evidence and still
	// suppress, since 100% of generic-tag assignments carry lexical evidence.
	tagger := &evidenceTagger{
		scores: []ai.TagScore{{Tag: "backend", Relevance: 0.5, Evidence: []string{"raw text"}}},
	}
	enricher := newEvidenceEnricher(t, tagger, []issues.Tag{
		{Name: "backend", Embedding: []float64{-1, 0}},
	})

	result, err := enricher.AnalyzeText(context.Background(), "raw text with search box clears", AnalyzeTextOptions{
		CandidateMode: tags.CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}

	backend := findScore(t, result.TagScores, "backend")
	if backend.VerificationVerdict != domain.TagVerificationVerdictDownRank {
		t.Fatalf("expected anti-alignment to override evidence (DownRank), got %q", backend.VerificationVerdict)
	}
	if backend.Negation == nil || *backend.Negation != verifierAntiAlignmentNegation {
		t.Fatalf("expected anti-alignment negation, got %#v", backend.Negation)
	}
}

func TestVerifierSuppressesCenteredAntiAlignedTag(t *testing.T) {
	// backend's RAW cosine with the issue is positive (~0.89), so raw-space checks
	// keep it — this is the anisotropy that makes raw suppression a no-op. After
	// centering it points opposite the issue (the real R² drag), so the centered
	// suppression fires.
	tagger := &evidenceTagger{scores: []ai.TagScore{{Tag: "backend", Relevance: 0.5}}}
	makeEnricher := func() *IssueEnricher {
		return newEvidenceEnricher(t, tagger, []issues.Tag{{Name: "backend", Embedding: []float64{2, 1}}})
	}
	opts := AnalyzeTextOptions{CandidateMode: tags.CandidateModeRetrievalShortlist, Verify: true}

	// With centering, the tag is suppressed.
	centered := makeEnricher()
	centered.UseCentering(fixedCenterer{means: issuemath.CorpusMeans{Issue: []float64{1.5, 0.5}, Tag: []float64{1.5, 0.5}}})
	result, err := centered.AnalyzeText(context.Background(), "raw text with search box clears", opts)
	if err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}
	backend := findScore(t, result.TagScores, "backend")
	if backend.VerificationVerdict != domain.TagVerificationVerdictDownRank {
		t.Fatalf("expected centered suppression (DownRank), got %q", backend.VerificationVerdict)
	}
	if backend.Negation == nil || *backend.Negation != verifierAntiAlignmentNegation {
		t.Fatalf("expected anti-alignment negation, got %#v", backend.Negation)
	}

	// Without centering, raw cosine stays positive → not suppressed.
	raw := makeEnricher()
	result2, err := raw.AnalyzeText(context.Background(), "raw text with search box clears", opts)
	if err != nil {
		t.Fatalf("AnalyzeText control: %v", err)
	}
	backend2 := findScore(t, result2.TagScores, "backend")
	if backend2.Negation != nil {
		t.Fatalf("expected no suppression without centering, got negation %v", *backend2.Negation)
	}
}

func TestAnalyzerNegationOverridesVerifierDominanceOnSameTag(t *testing.T) {
	// cleanup is dominated by database (sets verifier-dominance negation), and
	// the analyzer also emits a negation for cleanup with explicit evidence.
	// The analyzer signal should win, and the merged value should be the max
	// of the two, capped at 0.7. Relevance kept < 0.4 to route to DownRank
	// rather than Flagged.
	tagger := &evidenceTagger{
		scores: []ai.TagScore{{Tag: "cleanup", Relevance: 0.3}},
		negated: []ai.NegatedTag{
			{Tag: "cleanup", Confidence: 0.4, Evidence: []string{"search box clears"}},
		},
	}
	enricher := newEvidenceEnricher(t, tagger, []issues.Tag{
		{Name: "cleanup", Embedding: []float64{0, 1}},
		{Name: "database", Embedding: []float64{1, 0}},
	})

	result, err := enricher.AnalyzeText(context.Background(), "raw text with search box clears", AnalyzeTextOptions{
		CandidateMode: tags.CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}

	cleanup := findScore(t, result.TagScores, "cleanup")
	if cleanup.NegationProvenance != domain.NegationProvenanceAnalyzer {
		t.Fatalf("expected analyzer provenance to win, got %q", cleanup.NegationProvenance)
	}
	if cleanup.Negation == nil {
		t.Fatalf("expected merged Negation to be set")
	}
	// verifierDominanceNegation = 0.25, analyzer confidence = 0.4. Max = 0.4.
	if *cleanup.Negation != 0.4 {
		t.Fatalf("expected merged negation to take max (0.4), got %v", *cleanup.Negation)
	}
	if len(cleanup.NegationEvidence) == 0 {
		t.Fatalf("expected evidence ranges from analyzer signal")
	}
}

func TestAnalyzeTextNegationBelowConfidenceFloorDiscarded(t *testing.T) {
	tagger := &evidenceTagger{
		scores: []ai.TagScore{{Tag: "database", Relevance: 0.9, Evidence: []string{"raw text"}}},
		negated: []ai.NegatedTag{
			{Tag: "cleanup", Confidence: 0.05, Evidence: []string{"search box clears"}},
		},
	}
	enricher := newEvidenceEnricher(t, tagger, []issues.Tag{
		{Name: "database", Embedding: []float64{1, 0}},
		{Name: "cleanup", Embedding: []float64{0, 1}},
	})

	result, err := enricher.AnalyzeText(context.Background(), "raw text with search box clears", AnalyzeTextOptions{
		CandidateMode: tags.CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}

	for _, score := range result.TagScores {
		if score.Tag == "cleanup" {
			t.Fatalf("expected sub-floor confidence negation to be discarded, got %#v", score)
		}
	}
}
