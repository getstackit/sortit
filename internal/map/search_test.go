package issuemap

import (
	"testing"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

func TestSearchExcludesMergedAndDuplicateIssues(t *testing.T) {
	storeIssues := []issues.Issue{
		{ID: "canonical", Raw: "the canonical issue about billing", Tags: []string{"billing"}},
		{ID: "merged", Raw: "billing issue merged into canonical", Tags: []string{"billing"},
			Links: []issues.IssueLink{
				{Type: issues.IssueLinkTypeMergedInto, SourceIssueID: "merged", TargetIssueID: "canonical"},
			},
		},
		{ID: "duplicate", Raw: "duplicate billing issue", Tags: []string{"billing"},
			Links: []issues.IssueLink{
				{Type: issues.IssueLinkTypeDuplicateOf, SourceIssueID: "duplicate", TargetIssueID: "canonical"},
			},
		},
		{ID: "related", Raw: "related billing concern", Tags: []string{"billing"},
			Links: []issues.IssueLink{
				{Type: issues.IssueLinkTypeRelatedTo, SourceIssueID: "related", TargetIssueID: "canonical"},
			},
		},
	}

	tags := []issues.Tag{
		{Name: "billing", Embedding: []float64{1, 0}},
	}

	resp := SearchFromQueryWithTags(storeIssues, tags, "billing", nil, nil, 10)

	ids := make(map[string]bool, len(resp.RelatedIssues))
	for _, ri := range resp.RelatedIssues {
		ids[ri.ID] = true
	}

	if ids["merged"] {
		t.Error("merged issue should be excluded from search results")
	}
	if ids["duplicate"] {
		t.Error("duplicate issue should be excluded from search results")
	}
	if !ids["canonical"] {
		t.Error("canonical issue should appear in search results")
	}
	if !ids["related"] {
		t.Error("related issue should appear in search results")
	}
}

func TestSearchTagsDeprioritizesGenericBucketTags(t *testing.T) {
	lowSpecificity := 0.1
	highSpecificity := 0.8
	tags := []issues.Tag{
		{Name: "backend", Embedding: []float64{1, 0}, Specificity: &lowSpecificity},
		{Name: "billing", Embedding: []float64{0.98, 0.02}, Specificity: &highSpecificity},
		{Name: "payments", Embedding: []float64{0.96, 0.04}, Specificity: &highSpecificity},
	}

	results := SearchTags(tags, []float64{1, 0}, 3)
	if len(results) != 3 {
		t.Fatalf("expected 3 related tags, got %d", len(results))
	}

	if results[0].Name != "billing" {
		t.Fatalf("expected specific tag to rank first, got %q", results[0].Name)
	}
	if results[2].Name != "backend" {
		t.Fatalf("expected generic bucket tag to be deprioritized, got ordering %+v", results)
	}
}

func TestGenericQueryBoostsIssuesWithSpecificTags(t *testing.T) {
	lowSpecificity := 0.1
	highSpecificity := 0.8

	genericOnly := issues.Issue{
		ID:   "generic-only",
		Raw:  "backend service is slow",
		Tags: []string{"backend"},
		TagScores: []domain.TagRelevance{
			{Tag: "backend", Relevance: 0.9},
		},
	}
	withSpecific := issues.Issue{
		ID:   "with-specific",
		Raw:  "backend billing endpoint timeout",
		Tags: []string{"backend", "billing"},
		TagScores: []domain.TagRelevance{
			{Tag: "backend", Relevance: 0.8},
			{Tag: "billing", Relevance: 0.7},
		},
	}

	storeIssues := []issues.Issue{genericOnly, withSpecific}
	storeTags := []issues.Tag{
		{Name: "backend", Embedding: []float64{1, 0}, Specificity: &lowSpecificity},
		{Name: "billing", Embedding: []float64{0, 1}, Specificity: &highSpecificity},
	}

	resp := SearchFromQueryWithTags(storeIssues, storeTags, "backend", nil, nil, 10)

	if len(resp.RelatedIssues) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(resp.RelatedIssues))
	}

	if resp.RelatedIssues[0].ID != "with-specific" {
		t.Errorf("expected issue with specific co-occurring tags to rank first when searching by generic tag, got %q first", resp.RelatedIssues[0].ID)
	}
}

func TestRuntimeStoredTagRelevancesPreservesSignedMetadata(t *testing.T) {
	alignment := 0.42
	specificity := 0.71
	dominanceGap := 0.18
	negation := 0.63
	issue := issues.Issue{TagScores: []domain.TagRelevance{{
		Tag:                 " auth ",
		Relevance:           0.2,
		Suggested:           true,
		Description:         "authentication",
		CandidateSources:    []string{"analyzer"},
		Alignment:           &alignment,
		Specificity:         &specificity,
		VerificationVerdict: domain.TagVerificationVerdictFlagged,
		VerificationReason:  "better explained by billing",
		DominatedBy:         "billing",
		DominanceGap:        &dominanceGap,
		Evidence:            []domain.EvidenceRange{{Start: 1, End: 5, Text: "auth"}},
		Negation:            &negation,
		NegationProvenance:  domain.NegationProvenanceVerifier,
		NegationEvidence:    []domain.EvidenceRange{{Start: 7, End: 11, Text: "no auth"}},
		NegationReason:      "explicitly ruled out",
	}}}

	got := runtimeStoredTagRelevances(issue)
	if len(got) != 1 {
		t.Fatalf("expected 1 tag relevance, got %d", len(got))
	}
	tag := got[0]
	if tag.Tag != "auth" {
		t.Fatalf("expected trimmed tag name, got %q", tag.Tag)
	}
	if tag.Negation == nil || *tag.Negation != negation {
		t.Fatalf("expected negation %v, got %+v", negation, tag.Negation)
	}
	if tag.NegationProvenance != domain.NegationProvenanceVerifier || tag.NegationReason != "explicitly ruled out" {
		t.Fatalf("negation metadata was not preserved: %+v", tag)
	}
	if tag.VerificationVerdict != domain.TagVerificationVerdictFlagged || tag.DominatedBy != "billing" {
		t.Fatalf("verification metadata was not preserved: %+v", tag)
	}
	if len(tag.CandidateSources) != 1 || tag.CandidateSources[0] != "analyzer" {
		t.Fatalf("candidate sources were not preserved: %+v", tag.CandidateSources)
	}
	if tag.Negation == issue.TagScores[0].Negation || tag.Alignment == issue.TagScores[0].Alignment {
		t.Fatal("expected runtime tag relevance to copy pointer fields")
	}
}

func TestSearchQueryTagsPreservesSignedMetadata(t *testing.T) {
	negation := 0.55
	got := searchQueryTags([]domain.TagRelevance{
		{Tag: "billing", Relevance: 0.9},
		{
			Tag:                " auth ",
			Relevance:          0.4,
			Negation:           &negation,
			NegationProvenance: domain.NegationProvenanceAnalyzer,
			NegationReason:     "query says not auth",
		},
	})
	if len(got) != 2 {
		t.Fatalf("expected 2 query tags, got %d", len(got))
	}
	auth := got[1]
	if auth.Tag != "auth" {
		t.Fatalf("expected trimmed auth tag after relevance sort, got %+v", got)
	}
	if auth.Negation == nil || *auth.Negation != negation {
		t.Fatalf("expected query negation %v, got %+v", negation, auth.Negation)
	}
	if auth.NegationProvenance != domain.NegationProvenanceAnalyzer || auth.NegationReason != "query says not auth" {
		t.Fatalf("query negation metadata was not preserved: %+v", auth)
	}
}

func TestQueryMatchesMultiWordTagName(t *testing.T) {
	if !queryMatchesTagNames("front end regression", []string{"front end"}) {
		t.Fatal("expected query to match multi-word tag phrase")
	}
	if queryMatchesTagNames("frontend regression", []string{"front end"}) {
		t.Fatal("expected compact token not to match multi-word tag phrase")
	}
	if queryMatchesTagNames("front billing end", []string{"front end"}) {
		t.Fatal("expected non-contiguous words not to match tag phrase")
	}
}

func TestGenericQueryAndSpecificityUseNormalizedTagNames(t *testing.T) {
	lowSpecificity := 0.1
	tagSpecificity := buildTagSpecificityMap([]issues.Tag{
		{Name: "Front End", Specificity: &lowSpecificity},
	})

	if !queryMatchesGenericTag("front end regression", tagSpecificity) {
		t.Fatal("expected query to match generic multi-word tag")
	}
	if queryMatchesGenericTag("front billing end", tagSpecificity) {
		t.Fatal("expected non-contiguous words not to match generic tag phrase")
	}

	penalty := issueSpecificityPenalty([]domain.TagRelevance{{Tag: "front end", Relevance: 0.9}}, tagSpecificity)
	if penalty <= specificityPenalty(nil) {
		t.Fatalf("expected normalized specificity lookup to use low specificity; got penalty %v", penalty)
	}
}

func TestSearchUsesContentConfidenceAsNearTieBreaker(t *testing.T) {
	storeIssues := []issues.Issue{
		{
			ID:        "issue-vague",
			Raw:       "export bug",
			Status:    issues.StatusOpen,
			TagScores: []domain.TagRelevance{{Tag: "export", Relevance: 0.9}},
			Embedding: []float64{1, 0},
		},
		{
			ID:     "issue-detailed",
			Raw:    "Safari PDF export fails after tapping Share twice.\n1. Open the Share sheet.\n2. Tap Export as PDF.\nError: WebKit export exception in /app/export/pdf.ts:42.",
			Status: issues.StatusOpen,
			TagScores: []domain.TagRelevance{
				{Tag: "export", Relevance: 0.9},
			},
			Embedding: []float64{1, 0},
		},
	}

	storeTags := []issues.Tag{
		{Name: "export", Embedding: []float64{1, 0}},
	}

	resp := SearchFromQueryWithTags(storeIssues, storeTags, "export", nil, []float64{1, 0}, 10)
	if len(resp.RelatedIssues) != 2 {
		t.Fatalf("expected 2 related issues, got %d", len(resp.RelatedIssues))
	}
	if resp.RelatedIssues[0].ID != "issue-detailed" {
		t.Fatalf("expected detailed issue to win near-tie by content confidence, got %+v", resp.RelatedIssues)
	}
}

func TestSearchBoostsRecentlyActiveIssues(t *testing.T) {
	activeVelocity := 0.9
	storeIssues := []issues.Issue{
		{
			ID:        "issue-dormant",
			Raw:       "Safari export fails when generating a PDF attachment",
			Status:    issues.StatusOpen,
			TagScores: []domain.TagRelevance{{Tag: "export", Relevance: 0.9}},
			Embedding: []float64{1, 0},
		},
		{
			ID:        "issue-active",
			Raw:       "Safari export fails when generating a PDF attachment",
			Status:    issues.StatusOpen,
			TagScores: []domain.TagRelevance{{Tag: "export", Relevance: 0.9}},
			Embedding: []float64{1, 0},
			LifecycleMetrics: &issues.IssueLifecycleMetrics{
				Velocity: &activeVelocity,
			},
		},
	}

	storeTags := []issues.Tag{
		{Name: "export", Embedding: []float64{1, 0}},
	}

	resp := SearchFromQueryWithTags(storeIssues, storeTags, "export pdf attachment", nil, []float64{1, 0}, 10)
	if len(resp.RelatedIssues) != 2 {
		t.Fatalf("expected 2 related issues, got %d", len(resp.RelatedIssues))
	}
	if resp.RelatedIssues[0].ID != "issue-active" {
		t.Fatalf("expected active issue to rank first, got %+v", resp.RelatedIssues)
	}
}

func TestSearchPrefersFreshIssueWhenBaseSimilarityIsEqual(t *testing.T) {
	now := time.Now().UTC()
	storeIssues := []issues.Issue{
		{
			ID:        "issue-stale",
			Raw:       "Safari export fails when generating a PDF attachment",
			Status:    issues.StatusOpen,
			CreatedAt: now.Add(-240 * 24 * time.Hour),
			TagScores: []domain.TagRelevance{{Tag: "export", Relevance: 0.9}},
			Embedding: []float64{1, 0},
		},
		{
			ID:        "issue-fresh",
			Raw:       "Safari export fails when generating a PDF attachment",
			Status:    issues.StatusOpen,
			CreatedAt: now.Add(-2 * 24 * time.Hour),
			TagScores: []domain.TagRelevance{{Tag: "export", Relevance: 0.9}},
			Embedding: []float64{1, 0},
		},
	}

	storeTags := []issues.Tag{{Name: "export", Embedding: []float64{1, 0}}}

	resp := SearchFromQueryWithTags(storeIssues, storeTags, "export pdf attachment", nil, []float64{1, 0}, 10)
	if len(resp.RelatedIssues) != 2 {
		t.Fatalf("expected 2 related issues, got %d", len(resp.RelatedIssues))
	}
	if resp.RelatedIssues[0].ID != "issue-fresh" {
		t.Fatalf("expected fresh issue to rank first, got %+v", resp.RelatedIssues)
	}
}

func TestSearchReportsRawSemanticSimilarityWhenDecompositionIsActive(t *testing.T) {
	storeIssues := []issues.Issue{
		{ID: "semantic-only", Raw: "same words", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "beta", Relevance: 1}}, Embedding: unitVec([]float64{1, 0, 0, 0})},
		{ID: "alpha-a", Raw: "alpha issue a", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec([]float64{1, 0, 0, 0})},
		{ID: "alpha-b", Raw: "alpha issue b", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec([]float64{1, 0, 0, 0})},
		{ID: "alpha-c", Raw: "alpha issue c", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec([]float64{1, 0, 0, 0})},
		{ID: "alpha-d", Raw: "alpha issue d", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec([]float64{1, 0, 0, 0})},
	}
	storeTags := []issues.Tag{
		{Name: "alpha", Embedding: unitVec([]float64{1, 0, 0, 0})},
		{Name: "beta", Embedding: unitVec([]float64{0, 1, 0, 0})},
	}

	resp := SearchFromQueryWithTags(
		storeIssues,
		storeTags,
		"same words",
		[]domain.TagRelevance{{Tag: "alpha", Relevance: 1}},
		unitVec([]float64{1, 0, 0, 0}),
		10,
	)

	var found *RelatedIssue
	for i := range resp.RelatedIssues {
		if resp.RelatedIssues[i].ID == "semantic-only" {
			found = &resp.RelatedIssues[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected semantic-only issue in results, got %+v", resp.RelatedIssues)
	}
	if found.SemanticSimilarity != 1 {
		t.Fatalf("expected raw semantic similarity of 1.00, got %v", found.SemanticSimilarity)
	}
	if found.Reason != ReasonSemanticSimilar {
		t.Fatalf("expected semantic reason text, got %q", found.Reason)
	}
}

func TestSearchWithRidgeSimilarityRanksSameTagIssuesFirst(t *testing.T) {
	// Two tag clusters with separable embeddings; query leans on alpha.
	storeIssues := []issues.Issue{
		{ID: "alpha-a", Raw: "alpha issue a", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.9}}, Embedding: unitVec([]float64{0.95, 0.1, 0.1, 0})},
		{ID: "alpha-b", Raw: "alpha issue b", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.85}}, Embedding: unitVec([]float64{0.9, 0.2, 0.1, 0})},
		{ID: "alpha-c", Raw: "alpha issue c", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.8}}, Embedding: unitVec([]float64{0.92, 0.1, 0.2, 0})},
		{ID: "beta-a", Raw: "beta issue a", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "beta", Relevance: 0.9}}, Embedding: unitVec([]float64{0.1, 0.95, 0.1, 0})},
		{ID: "beta-b", Raw: "beta issue b", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "beta", Relevance: 0.85}}, Embedding: unitVec([]float64{0.2, 0.9, 0.1, 0})},
		{ID: "beta-c", Raw: "beta issue c", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "beta", Relevance: 0.8}}, Embedding: unitVec([]float64{0.1, 0.92, 0.2, 0})},
	}
	storeTags := []issues.Tag{
		{Name: "alpha", Embedding: unitVec([]float64{1, 0, 0, 0})},
		{Name: "beta", Embedding: unitVec([]float64{0, 1, 0, 0})},
	}

	resp := SearchFromQueryWithTags(
		storeIssues, storeTags,
		"alpha problem",
		[]domain.TagRelevance{{Tag: "alpha", Relevance: 0.9}},
		unitVec([]float64{0.95, 0.1, 0.1, 0}),
		3,
		WithRidgeSimilarity(1.0),
	)

	if len(resp.RelatedIssues) != 3 {
		t.Fatalf("expected 3 results, got %d", len(resp.RelatedIssues))
	}
	for _, r := range resp.RelatedIssues {
		if r.ID[:5] != "alpha" {
			t.Errorf("ridge ranking surfaced a non-alpha issue %q in the top 3 for an alpha query", r.ID)
		}
	}
}

func TestSearchWithRidgeSimilarityFallsBackOnTinyCorpus(t *testing.T) {
	// Below MinDecompositionIssues the ridge decomposition can't run; the
	// option must degrade to the default model without error.
	storeIssues := []issues.Issue{
		{ID: "a", Raw: "alpha one", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.9}}, Embedding: unitVec([]float64{1, 0, 0, 0})},
		{ID: "b", Raw: "alpha two", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.8}}, Embedding: unitVec([]float64{0.9, 0.1, 0, 0})},
	}
	storeTags := []issues.Tag{
		{Name: "alpha", Embedding: unitVec([]float64{1, 0, 0, 0})},
		{Name: "beta", Embedding: unitVec([]float64{0, 1, 0, 0})},
	}

	resp := SearchFromQueryWithTags(
		storeIssues, storeTags,
		"alpha",
		[]domain.TagRelevance{{Tag: "alpha", Relevance: 0.9}},
		unitVec([]float64{1, 0, 0, 0}),
		10,
		WithRidgeSimilarity(1.0),
	)
	if len(resp.RelatedIssues) == 0 {
		t.Fatal("expected results from the rank-1 fallback on a tiny corpus")
	}
}

func TestCandidateInRegionRespectsMembershipFloor(t *testing.T) {
	tags := []domain.TagRelevance{
		{Tag: "auth", Relevance: 0.5},
		{Tag: "billing", Relevance: 0.3},
	}
	if !candidateInRegion(tags, "auth") {
		t.Fatal("expected auth at 0.5 to clear membership floor")
	}
	if candidateInRegion(tags, "billing") {
		t.Fatal("expected billing at 0.3 to fall below membership floor")
	}
}

func TestCandidateAntiCorrelationPenaltySumsStrongTagWeights(t *testing.T) {
	tags := []domain.TagRelevance{
		{Tag: "ui", Relevance: 0.8},
		{Tag: "ops", Relevance: 0.2}, // below strong-tag floor; ignored
	}
	anti := map[string]float64{
		"ui":  0.5, // 0.5 * 0.12 = 0.06
		"ops": 0.9, // ignored — relevance too low
	}
	penalty := candidateAntiCorrelationPenalty(tags, anti)
	want := 0.06
	if penalty <= want-0.0001 || penalty >= want+0.0001 {
		t.Fatalf("penalty = %v, want %v", penalty, want)
	}
}
