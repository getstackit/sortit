package issuemap

import (
	"testing"

	"splat/internal/domain"
	"splat/internal/issues"
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
