package services

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"splat/internal/ai"
	"splat/internal/issues"
)

type catalogTestStore struct {
	tags               []issues.Tag
	upserted           []issues.Tag
	specificityUpdates []catalogSpecificityUpdate
}

type catalogSpecificityUpdate struct {
	name        string
	specificity *float64
	llm         *float64
	embedding   *float64
	computedAt  *time.Time
}

func (s *catalogTestStore) ListTags(context.Context) ([]issues.Tag, error) {
	return append([]issues.Tag(nil), s.tags...), nil
}

func (s *catalogTestStore) UpsertTags(_ context.Context, tags []issues.Tag) error {
	s.upserted = append([]issues.Tag(nil), tags...)
	s.tags = append([]issues.Tag(nil), tags...)
	return nil
}

func (s *catalogTestStore) UpdateTagSpecificity(_ context.Context, name string, specificity, llm, embedding *float64, computedAt *time.Time) error {
	s.specificityUpdates = append(s.specificityUpdates, catalogSpecificityUpdate{
		name:        name,
		specificity: cloneFloat64Pointer(specificity),
		llm:         cloneFloat64Pointer(llm),
		embedding:   cloneFloat64Pointer(embedding),
		computedAt:  cloneTimePointer(computedAt),
	})
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
		slog.Default(),
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
		slog.Default(),
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

func TestComputeEmbeddingSpecificityIsOrderInvariantByTagName(t *testing.T) {
	first := []issues.Tag{
		{Name: "backend", Embedding: []float64{1, 0, 0}},
		{Name: "billing", Embedding: []float64{0.99, 0.01, 0}},
		{Name: "payments", Embedding: []float64{0.98, 0.02, 0}},
		{Name: "safari", Embedding: []float64{0, 1, 0}},
	}
	second := []issues.Tag{
		first[3],
		first[1],
		first[0],
		first[2],
	}

	firstScores := computeEmbeddingSpecificity(first)
	secondScores := computeEmbeddingSpecificity(second)
	for _, name := range []string{"backend", "billing", "payments", "safari"} {
		assertFloatPointerEqual(t, name, firstScores[name], secondScores[name])
	}
}

func TestScoreAllTagsSpecificityPersistsCanonicalDeterministicScore(t *testing.T) {
	store := &catalogTestStore{
		tags: []issues.Tag{
			{Name: "backend", Embedding: []float64{1, 0, 0}},
			{Name: "billing", Embedding: []float64{0.99, 0.01, 0}},
			{Name: "payments", Embedding: []float64{0.98, 0.02, 0}},
			{Name: "safari", Embedding: []float64{0, 1, 0}},
			{Name: "empty"},
		},
	}
	service := NewCatalogService(store, nil, slog.Default())

	if err := service.ScoreAllTagsSpecificity(context.Background()); err != nil {
		t.Fatalf("ScoreAllTagsSpecificity: %v", err)
	}

	if len(store.specificityUpdates) != 5 {
		t.Fatalf("expected 5 specificity updates, got %d", len(store.specificityUpdates))
	}

	for _, name := range []string{"backend", "billing", "payments", "safari"} {
		update := findSpecificityUpdate(t, store.specificityUpdates, name)
		if update.specificity == nil {
			t.Fatalf("%s specificity = nil, want non-nil", name)
		}
		if update.llm != nil {
			t.Fatalf("%s llm = %v, want nil", name, *update.llm)
		}
		assertFloatPointerEqual(t, name, update.specificity, update.embedding)
		if update.computedAt == nil {
			t.Fatalf("%s computedAt = nil, want non-nil", name)
		}
	}

	empty := findSpecificityUpdate(t, store.specificityUpdates, "empty")
	if empty.specificity != nil {
		t.Fatalf("empty specificity = %v, want nil", *empty.specificity)
	}
	if empty.embedding != nil {
		t.Fatalf("empty embedding = %v, want nil", *empty.embedding)
	}
	if empty.llm != nil {
		t.Fatalf("empty llm = %v, want nil", *empty.llm)
	}
}

func TestScoreTagSpecificityClearsScoreForSmallCatalog(t *testing.T) {
	store := &catalogTestStore{
		tags: []issues.Tag{
			{Name: "backend", Embedding: []float64{1, 0, 0}},
			{Name: "billing", Embedding: []float64{0.99, 0.01, 0}},
			{Name: "safari", Embedding: []float64{0, 1, 0}},
		},
	}
	service := NewCatalogService(store, nil, slog.Default())

	if err := service.ScoreTagSpecificity(context.Background(), "backend"); err != nil {
		t.Fatalf("ScoreTagSpecificity: %v", err)
	}

	update := findSpecificityUpdate(t, store.specificityUpdates, "backend")
	if update.specificity != nil {
		t.Fatalf("backend specificity = %v, want nil for small catalog", *update.specificity)
	}
	if update.embedding != nil {
		t.Fatalf("backend embedding = %v, want nil for small catalog", *update.embedding)
	}
	if update.llm != nil {
		t.Fatalf("backend llm = %v, want nil", *update.llm)
	}
}

func findSpecificityUpdate(t *testing.T, updates []catalogSpecificityUpdate, name string) catalogSpecificityUpdate {
	t.Helper()

	for _, update := range updates {
		if update.name == name {
			return update
		}
	}
	t.Fatalf("missing specificity update for %q", name)
	return catalogSpecificityUpdate{}
}

func assertFloatPointerEqual(t *testing.T, label string, left, right *float64) {
	t.Helper()

	switch {
	case left == nil && right == nil:
		return
	case left == nil || right == nil:
		t.Fatalf("%s pointer mismatch: %v vs %v", label, left, right)
	}
	if *left != *right {
		t.Fatalf("%s value mismatch: %v vs %v", label, *left, *right)
	}
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
