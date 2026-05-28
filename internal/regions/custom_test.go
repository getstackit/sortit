package regions

import (
	"testing"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

func TestMatchesPredicateTagFilter(t *testing.T) {
	now := time.Now()
	issue := issues.Issue{
		Status:    issues.StatusOpen,
		TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.8}},
	}
	predicate := domain.CustomRegionPredicate{
		Tags: []domain.TagPredicate{{Tag: "auth", MinRelevance: 0.5}},
	}
	if !MatchesPredicate(issue, predicate, now) {
		t.Fatal("expected match for tag at floor")
	}

	predicate.Tags[0].MinRelevance = 0.9
	if MatchesPredicate(issue, predicate, now) {
		t.Fatal("expected no match when relevance below floor")
	}
}

func TestMatchesPredicateStatusFilter(t *testing.T) {
	now := time.Now()
	open := issues.Issue{Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.8}}}
	closed := issues.Issue{Status: issues.StatusClosed, TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.8}}}
	predicate := domain.CustomRegionPredicate{
		Tags:     []domain.TagPredicate{{Tag: "auth", MinRelevance: 0.5}},
		Statuses: []string{"open"},
	}
	if !MatchesPredicate(open, predicate, now) {
		t.Fatal("expected open issue to match")
	}
	if MatchesPredicate(closed, predicate, now) {
		t.Fatal("expected closed issue to be filtered out")
	}
}

func TestMatchesPredicateAgeFilter(t *testing.T) {
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	young := issues.Issue{CreatedAt: now.Add(-5 * 24 * time.Hour), TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.8}}}
	old := issues.Issue{CreatedAt: now.Add(-60 * 24 * time.Hour), TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.8}}}
	maxAge := 30
	predicate := domain.CustomRegionPredicate{
		Tags:       []domain.TagPredicate{{Tag: "auth", MinRelevance: 0.5}},
		MaxAgeDays: &maxAge,
	}
	if !MatchesPredicate(young, predicate, now) {
		t.Fatal("expected 5-day-old issue to match 30-day limit")
	}
	if MatchesPredicate(old, predicate, now) {
		t.Fatal("expected 60-day-old issue to be filtered out")
	}
}

func TestMatchesPredicateAllEmptyMatchesEverything(t *testing.T) {
	issue := issues.Issue{Status: issues.StatusOpen}
	if !MatchesPredicate(issue, domain.CustomRegionPredicate{}, time.Now()) {
		t.Fatal("expected empty predicate to match all issues")
	}
}
