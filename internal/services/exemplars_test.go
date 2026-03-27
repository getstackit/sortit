package services

import (
	"context"
	"testing"

	"splat/internal/ai"
)

func TestExemplarPoolSelectReturnsMostSimilar(t *testing.T) {
	embedder := ai.NewStubEmbedder()

	pool := NewExemplarPool([]ai.FewShotExample{
		{Text: "frontend button layout issue"},
		{Text: "database migration schema change"},
		{Text: "api endpoint timeout error"},
	})

	// Embed a query text that is close to the database example.
	result, err := embedder.EmbedText(context.Background(), "database schema migration")
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}
	queryEmbedding := Float32VectorToFloat64(result.Vector)

	selected := pool.Select(context.Background(), embedder, queryEmbedding, nil, 2)
	if len(selected) != 2 {
		t.Fatalf("expected 2 exemplars, got %d", len(selected))
	}
	// The first result should be the most similar — the database example.
	if selected[0].Text != "database migration schema change" {
		t.Errorf("expected database example first, got %q", selected[0].Text)
	}
}

func TestExemplarPoolSelectPrefersSharedTags(t *testing.T) {
	embedder := ai.NewStubEmbedder()

	pool := NewExemplarPool([]ai.FewShotExample{
		{
			Text: "frontend button issue",
			Tags: []ai.FewShotTag{{Name: "frontend", Relevance: 0.9}},
		},
		{
			Text: "api timeout error",
			Tags: []ai.FewShotTag{{Name: "backend", Relevance: 0.9}},
		},
	})

	result, err := embedder.EmbedText(context.Background(), "some query")
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}
	queryEmbedding := Float32VectorToFloat64(result.Vector)

	// When candidate tags include "backend", prefer the backend example.
	selected := pool.Select(context.Background(), embedder, queryEmbedding, []string{"backend"}, 1)
	if len(selected) != 1 {
		t.Fatalf("expected 1 exemplar, got %d", len(selected))
	}
	if selected[0].Tags[0].Name != "backend" {
		t.Errorf("expected backend example, got %q", selected[0].Tags[0].Name)
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
