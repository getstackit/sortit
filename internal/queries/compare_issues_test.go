package queries

import (
	"context"
	"testing"

	"splat/internal/issues"
)

type compareOnlyStore struct {
	items []issues.CompareIssue
}

func (s compareOnlyStore) List(_ context.Context) ([]issues.Issue, error) {
	panic("List should not be called in compare handler")
}

func (s compareOnlyStore) Get(_ context.Context, _ string) (issues.Issue, error) {
	panic("Get should not be called when compare store is available")
}

func (s compareOnlyStore) SaveIssue(context.Context, issues.Issue) error { return nil }
func (s compareOnlyStore) SaveIssuePost(context.Context, issues.IssuePost) error {
	return nil
}
func (s compareOnlyStore) UpdateIssueFields(context.Context, string, issues.IssueFieldUpdate) error {
	return nil
}
func (s compareOnlyStore) SaveOperation(context.Context, issues.IssueOperation) error { return nil }
func (s compareOnlyStore) SaveLink(context.Context, issues.IssueLink) error           { return nil }

func (s compareOnlyStore) ListCompareIssues(_ context.Context, ids []string) ([]issues.CompareIssue, error) {
	items := make([]issues.CompareIssue, 0, len(ids))
	for _, id := range ids {
		for _, item := range s.items {
			if item.ID != id {
				continue
			}
			items = append(items, issues.CompareIssue{
				ID:        item.ID,
				Embedding: append([]float64(nil), item.Embedding...),
			})
			break
		}
	}
	return items, nil
}

func TestCompareIssuesUsesCompareStore(t *testing.T) {
	handler := CompareIssuesHandler{
		Store: compareOnlyStore{
			items: []issues.CompareIssue{
				{ID: "issue-a", Embedding: []float64{1, 0}},
				{ID: "issue-b", Embedding: []float64{0.6, 0.8}},
			},
		},
	}

	result, err := handler.Handle(context.Background(), CompareIssues{IDs: []string{"issue-a", "issue-b"}})
	if err != nil {
		t.Fatalf("handle compare issues: %v", err)
	}
	if result.ComparedIssueCount != 2 {
		t.Fatalf("expected 2 compared issues, got %d", result.ComparedIssueCount)
	}
	if result.AverageEmbeddingSimilarity != 0.6 {
		t.Fatalf("expected average similarity 0.6, got %v", result.AverageEmbeddingSimilarity)
	}
	if len(result.PairwiseEmbeddingSimilarity) != 1 {
		t.Fatalf("expected 1 pair, got %#v", result.PairwiseEmbeddingSimilarity)
	}
	if result.PairwiseEmbeddingSimilarity[0].Similarity != 0.6 {
		t.Fatalf("expected pair similarity 0.6, got %v", result.PairwiseEmbeddingSimilarity[0].Similarity)
	}
}
