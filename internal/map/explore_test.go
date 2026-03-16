package issuemap

import (
	"testing"
	"time"

	"splat/internal/issues"
)

func TestExploreOpportunitiesPreferMatureIssueGroups(t *testing.T) {
	highMaturity := 0.9
	lowMaturity := 0.2

	resp, err := ExploreFromIssuesWithTags(
		[]issues.Issue{
			{
				ID:     "issue-target",
				Raw:    "Safari export fails after tapping share twice",
				Status: issues.StatusOpen,
				TagScores: []issues.TagRelevance{
					{Tag: "export", Relevance: 0.9},
					{Tag: "safari", Relevance: 0.8},
				},
				Embedding:        []float64{1, 0},
				LifecycleMetrics: &issues.IssueLifecycleMetrics{Maturity: &highMaturity},
			},
			{
				ID:     "issue-mature",
				Raw:    "Safari export crashes on PDF download",
				Status: issues.StatusOpen,
				TagScores: []issues.TagRelevance{
					{Tag: "export", Relevance: 0.9},
				},
				Embedding:        []float64{1, 0},
				LifecycleMetrics: &issues.IssueLifecycleMetrics{Maturity: &highMaturity},
			},
			{
				ID:     "issue-immature",
				Raw:    "Safari rendering glitch after rotation",
				Status: issues.StatusOpen,
				TagScores: []issues.TagRelevance{
					{Tag: "safari", Relevance: 0.9},
				},
				Embedding:        []float64{0.85, 0.15},
				LifecycleMetrics: &issues.IssueLifecycleMetrics{Maturity: &lowMaturity},
			},
		},
		[]issues.Tag{
			{Name: "export", Embedding: []float64{1, 0}},
			{Name: "safari", Embedding: []float64{0.9, 0.1}},
		},
		"issue-target",
		10,
	)
	if err != nil {
		t.Fatalf("ExploreFromIssuesWithTags: %v", err)
	}
	if len(resp.Opportunities) == 0 {
		t.Fatal("expected opportunities")
	}
	if len(resp.Opportunities) < 2 {
		t.Fatalf("expected 2 opportunity groups, got %+v", resp.Opportunities)
	}
	if resp.Opportunities[0].IssueIDs[1] != "issue-mature" {
		t.Fatalf("expected mature issue opportunity first, got %+v", resp.Opportunities)
	}
	if resp.Opportunities[0].Confidence <= resp.Opportunities[1].Confidence {
		t.Fatalf("expected mature opportunity confidence %v > immature %v", resp.Opportunities[0].Confidence, resp.Opportunities[1].Confidence)
	}
}

func TestExploreOpportunitiesBoostHighVelocityClusters(t *testing.T) {
	steadyMaturity := 0.6
	highVelocity := 0.85
	lowVelocity := 0.05

	resp, err := ExploreFromIssuesWithTags(
		[]issues.Issue{
			{
				ID:     "issue-target",
				Raw:    "Safari export fails after tapping share twice",
				Status: issues.StatusOpen,
				TagScores: []issues.TagRelevance{
					{Tag: "export", Relevance: 0.9},
					{Tag: "safari", Relevance: 0.8},
				},
				Embedding: []float64{1, 0},
				LifecycleMetrics: &issues.IssueLifecycleMetrics{
					Maturity: &steadyMaturity,
					Velocity: &highVelocity,
				},
			},
			{
				ID:     "issue-active",
				Raw:    "Safari export crashes on PDF download",
				Status: issues.StatusOpen,
				TagScores: []issues.TagRelevance{
					{Tag: "export", Relevance: 0.9},
				},
				Embedding: []float64{1, 0},
				LifecycleMetrics: &issues.IssueLifecycleMetrics{
					Maturity: &steadyMaturity,
					Velocity: &highVelocity,
				},
			},
			{
				ID:     "issue-dormant",
				Raw:    "Safari rendering glitch after rotation",
				Status: issues.StatusOpen,
				TagScores: []issues.TagRelevance{
					{Tag: "safari", Relevance: 0.9},
				},
				Embedding: []float64{0.85, 0.15},
				LifecycleMetrics: &issues.IssueLifecycleMetrics{
					Maturity: &steadyMaturity,
					Velocity: &lowVelocity,
				},
			},
		},
		[]issues.Tag{
			{Name: "export", Embedding: []float64{1, 0}},
			{Name: "safari", Embedding: []float64{0.9, 0.1}},
		},
		"issue-target",
		10,
	)
	if err != nil {
		t.Fatalf("ExploreFromIssuesWithTags: %v", err)
	}
	if len(resp.Opportunities) < 2 {
		t.Fatalf("expected 2 opportunity groups, got %+v", resp.Opportunities)
	}
	if resp.Opportunities[0].IssueIDs[1] != "issue-active" {
		t.Fatalf("expected active opportunity first, got %+v", resp.Opportunities)
	}
	if resp.Opportunities[0].Confidence <= resp.Opportunities[1].Confidence {
		t.Fatalf("expected active opportunity confidence %v > dormant %v", resp.Opportunities[0].Confidence, resp.Opportunities[1].Confidence)
	}
}

func TestExplorePrefersFresherRelatedIssue(t *testing.T) {
	now := time.Now().UTC()
	resp, err := ExploreFromIssuesWithTags(
		[]issues.Issue{
			{
				ID:        "issue-target",
				Raw:       "Safari export fails after tapping share twice",
				Status:    issues.StatusOpen,
				CreatedAt: now.Add(-24 * time.Hour),
				TagScores: []issues.TagRelevance{
					{Tag: "export", Relevance: 0.9},
					{Tag: "safari", Relevance: 0.8},
				},
				Embedding: []float64{1, 0},
			},
			{
				ID:        "issue-fresh",
				Raw:       "Safari export crashes on PDF download",
				Status:    issues.StatusOpen,
				CreatedAt: now.Add(-48 * time.Hour),
				TagScores: []issues.TagRelevance{
					{Tag: "export", Relevance: 0.9},
				},
				Embedding: []float64{1, 0},
			},
			{
				ID:        "issue-stale",
				Raw:       "Safari export crashes on PDF download",
				Status:    issues.StatusOpen,
				CreatedAt: now.Add(-300 * 24 * time.Hour),
				TagScores: []issues.TagRelevance{
					{Tag: "export", Relevance: 0.9},
				},
				Embedding: []float64{1, 0},
			},
		},
		[]issues.Tag{{Name: "export", Embedding: []float64{1, 0}}, {Name: "safari", Embedding: []float64{0.9, 0.1}}},
		"issue-target",
		10,
	)
	if err != nil {
		t.Fatalf("ExploreFromIssuesWithTags: %v", err)
	}
	if len(resp.RelatedIssues) < 2 {
		t.Fatalf("expected 2 related issues, got %+v", resp.RelatedIssues)
	}
	if resp.RelatedIssues[0].ID != "issue-fresh" {
		t.Fatalf("expected fresher issue first, got %+v", resp.RelatedIssues)
	}
}
