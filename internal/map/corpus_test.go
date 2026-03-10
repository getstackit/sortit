package issuemap

import (
	"testing"

	"splat/internal/issues"
)

func TestSubsetDerivedCorpusFiltersWithoutRebuildingPositions(t *testing.T) {
	storeIssues := issues.FixtureIssues()
	corpus, err := BuildDerivedCorpus(storeIssues, nil)
	if err != nil {
		t.Fatalf("build corpus: %v", err)
	}

	keep := map[string]struct{}{
		storeIssues[0].ID: {},
		storeIssues[1].ID: {},
	}
	filtered := SubsetDerivedCorpus(corpus, keep)

	if len(filtered.Issues) != 2 {
		t.Fatalf("expected 2 filtered issues, got %d", len(filtered.Issues))
	}
	if len(filtered.RuntimeIssues) != 2 {
		t.Fatalf("expected 2 filtered runtime issues, got %d", len(filtered.RuntimeIssues))
	}
	if len(filtered.MapIssues) != 2 {
		t.Fatalf("expected 2 filtered map issues, got %d", len(filtered.MapIssues))
	}
	if len(filtered.IssueEmbeddings) != 2 {
		t.Fatalf("expected 2 issue embeddings, got %d", len(filtered.IssueEmbeddings))
	}
	if len(filtered.FactorVectors) != 2 {
		t.Fatalf("expected 2 factor vectors, got %d", len(filtered.FactorVectors))
	}
	for id := range keep {
		if filtered.Positions[id] != corpus.Positions[id] {
			t.Fatalf("expected filtered position for %s to match cached corpus", id)
		}
		if _, ok := filtered.IssuesByID[id]; !ok {
			t.Fatalf("expected issuesByID to include %s", id)
		}
		if _, ok := filtered.MapIssuesByID[id]; !ok {
			t.Fatalf("expected mapIssuesByID to include %s", id)
		}
	}
}

func TestSubsetDerivedCorpusEmptySet(t *testing.T) {
	corpus, err := BuildDerivedCorpus(issues.FixtureIssues(), nil)
	if err != nil {
		t.Fatalf("build corpus: %v", err)
	}

	filtered := SubsetDerivedCorpus(corpus, map[string]struct{}{})
	if len(filtered.Issues) != 0 {
		t.Fatalf("expected empty issues, got %d", len(filtered.Issues))
	}
	if len(filtered.VisibleIssueIDs) != 0 {
		t.Fatalf("expected no visible issues, got %d", len(filtered.VisibleIssueIDs))
	}
	if len(filtered.IssueEmbeddings) != 0 {
		t.Fatalf("expected no issue embeddings, got %d", len(filtered.IssueEmbeddings))
	}
	if len(filtered.Positions) != 0 {
		t.Fatalf("expected no positions, got %d", len(filtered.Positions))
	}
}
