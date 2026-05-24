package enrichment

import (
	"context"
	"testing"

	"sortit/internal/ai"
)

func TestExemplarPoolSelectReturnsMostSimilar(t *testing.T) {
	embedder := ai.NewStubEmbedder()

	pool := NewExemplarPool([]ai.FewShotExample{
		{Text: "frontend button layout issue"},
		{Text: "database migration schema change"},
		{Text: "api endpoint timeout error"},
	})

	result, err := embedder.EmbedText(context.Background(), "database schema migration")
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}
	queryEmbedding := Float32VectorToFloat64(result.Vector)

	selected := pool.Select(context.Background(), embedder, queryEmbedding, nil, 2)
	if len(selected) == 0 {
		t.Fatal("expected at least one exemplar")
	}
	if selected[0].Text != "database migration schema change" {
		t.Errorf("expected database example first, got %q", selected[0].Text)
	}
}

func TestExemplarPoolSelectPrefersSharedTags(t *testing.T) {
	pool := NewExemplarPool([]ai.FewShotExample{
		{
			Text:      "frontend button issue",
			Tags:      []ai.FewShotTag{{Name: "frontend", Relevance: 0.9}},
			Embedding: []float64{0.95, 0.05},
		},
		{
			Text:      "copy button on issue detail page",
			Tags:      []ai.FewShotTag{{Name: "issue-detail-page", Relevance: 0.9}},
			Embedding: []float64{0.8, 0.2},
		},
	})

	selected := pool.Select(context.Background(), nil, []float64{1, 0}, []string{"frontend", "issue-detail-page"}, 1)
	if len(selected) != 1 {
		t.Fatalf("expected 1 exemplar, got %d", len(selected))
	}
	if selected[0].Tags[0].Name != "issue-detail-page" {
		t.Errorf("expected specific shared-tag example, got %q", selected[0].Tags[0].Name)
	}
}

func TestExemplarPoolSkipsWeakBroadTagMatches(t *testing.T) {
	pool := NewExemplarPool([]ai.FewShotExample{
		{
			Text:      "frontend button issue",
			Tags:      []ai.FewShotTag{{Name: "frontend", Relevance: 0.9}},
			Embedding: []float64{0.1, 0.99},
		},
		{
			Text:      "bug in search results page",
			Tags:      []ai.FewShotTag{{Name: "bug", Relevance: 0.9}},
			Embedding: []float64{0, 1},
		},
	})

	selected := pool.Select(context.Background(), nil, []float64{1, 0}, []string{"frontend", "bug"}, 2)
	if len(selected) != 0 {
		t.Fatalf("expected weak broad-tag exemplars to be skipped, got %#v", selected)
	}
}

func TestExemplarPoolNilIsNoOp(t *testing.T) {
	var pool *ExemplarPool
	selected := pool.Select(context.Background(), nil, []float64{1, 0, 0}, nil, 3)
	if len(selected) != 0 {
		t.Fatalf("expected no exemplars from nil pool, got %d", len(selected))
	}
}

func TestExemplarPoolSelectRespectsLimit(t *testing.T) {
	embedder := ai.NewStubEmbedder()

	pool := NewExemplarPool([]ai.FewShotExample{
		{Text: "one"},
		{Text: "two"},
		{Text: "three"},
	})

	result, err := embedder.EmbedText(context.Background(), "query")
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}
	queryEmbedding := Float32VectorToFloat64(result.Vector)

	selected := pool.Select(context.Background(), embedder, queryEmbedding, nil, 1)
	if len(selected) != 1 {
		t.Fatalf("expected 1 exemplar, got %d", len(selected))
	}
}

func TestDefaultExemplarPoolIsNonEmpty(t *testing.T) {
	pool := DefaultExemplarPool()
	if pool == nil {
		t.Fatal("expected non-nil default pool")
	}
	if len(pool.items) == 0 {
		t.Fatal("expected non-empty default pool")
	}
}
