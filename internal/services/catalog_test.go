package services

import (
	"context"
	"testing"
	"time"

	"splat/internal/ai"
	"splat/internal/issues"
)

type catalogTestStore struct {
	tags     []issues.Tag
	upserted []issues.Tag
}

func (s *catalogTestStore) ListTags(context.Context) ([]issues.Tag, error) {
	return append([]issues.Tag(nil), s.tags...), nil
}

func (s *catalogTestStore) UpsertTags(_ context.Context, tags []issues.Tag) error {
	s.upserted = append([]issues.Tag(nil), tags...)
	s.tags = append([]issues.Tag(nil), tags...)
	return nil
}

func (s *catalogTestStore) UpdateTagSpecificity(_ context.Context, _ string, _, _, _ *float64, _ *time.Time) error {
	return nil
}

type catalogTestTagger struct{}

func (catalogTestTagger) Score(context.Context, string, []ai.Tag) ([]ai.TagScore, error) {
	return nil, nil
}

func (catalogTestTagger) Provider() string {
	return "test" //nolint:goconst
}

func (catalogTestTagger) Model() string {
	return "test" //nolint:goconst
}

type catalogTestEmbedder struct {
	calls []string
}

func (e *catalogTestEmbedder) EmbedText(_ context.Context, text string) (ai.EmbeddingResult, error) {
	e.calls = append(e.calls, text)
	return ai.EmbeddingResult{
		Vector: []float32{float32(len(e.calls)), 1},
	}, nil
}

func (e *catalogTestEmbedder) Provider() string {
	return "test"
}

func (e *catalogTestEmbedder) Model() string {
	return "test"
}

func TestEnsureStoredTagsReembedsWhenDescriptionChanges(t *testing.T) {
	store := &catalogTestStore{
		tags: []issues.Tag{
			{
				Name:        "export",
				Description: "old export wording",
				Embedding:   []float64{0.2, 0.8},
			},
		},
	}
	embedder := &catalogTestEmbedder{}
	service := NewCatalogService(
		store,
		ai.NewAnalyzer(catalogTestTagger{}, embedder),
	)

	err := service.EnsureStoredTags(context.Background(), []issues.Tag{
		{
			Name:        "export",
			Description: "download, file generation, sharing, or data extraction workflows. Excludes general browsing unless data leaves the product.",
		},
	})
	if err != nil {
		t.Fatalf("ensure stored tags: %v", err)
	}

	if len(embedder.calls) != 1 {
		t.Fatalf("expected exactly one re-embed call, got %d", len(embedder.calls))
	}
	if embedder.calls[0] != "export - download, file generation, sharing, or data extraction workflows. Excludes general browsing unless data leaves the product." {
		t.Fatalf("unexpected embed descriptor %q", embedder.calls[0])
	}
	if len(store.upserted) != 1 {
		t.Fatalf("expected one upserted tag, got %d", len(store.upserted))
	}
	if store.upserted[0].Description == "old export wording" {
		t.Fatalf("expected updated description to persist")
	}
	if len(store.upserted[0].Embedding) == 0 {
		t.Fatalf("expected updated embedding to persist")
	}
}

func TestEnsureStoredTagsKeepsExistingEmbeddingWhenDescriptionUnchanged(t *testing.T) {
	store := &catalogTestStore{
		tags: []issues.Tag{
			{
				Name:        "search",
				Description: "query entry, filtering, ranking, result retrieval, or finding content. Excludes unrelated navigation unless search behavior is central.",
				Embedding:   []float64{0.4, 0.6},
			},
		},
	}
	embedder := &catalogTestEmbedder{}
	service := NewCatalogService(
		store,
		ai.NewAnalyzer(catalogTestTagger{}, embedder),
	)

	err := service.EnsureStoredTags(context.Background(), []issues.Tag{
		{
			Name:        "search",
			Description: "query entry, filtering, ranking, result retrieval, or finding content. Excludes unrelated navigation unless search behavior is central.",
		},
	})
	if err != nil {
		t.Fatalf("ensure stored tags: %v", err)
	}

	if len(embedder.calls) != 0 {
		t.Fatalf("expected no re-embed call, got %d", len(embedder.calls))
	}
	if len(store.upserted) != 1 {
		t.Fatalf("expected one upserted tag, got %d", len(store.upserted))
	}
	if got := store.upserted[0].Embedding; len(got) != 2 || got[0] != 0.4 || got[1] != 0.6 {
		t.Fatalf("expected existing embedding to be preserved, got %#v", got)
	}
}
