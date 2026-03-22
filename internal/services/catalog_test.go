package services

import (
	"context"
	"log/slog"
	"slices"
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
	return testProviderName //nolint:goconst
}

func (catalogTestTagger) Model() string {
	return testProviderName //nolint:goconst
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
	return testProviderName
}

func (e *catalogTestEmbedder) Model() string {
	return testProviderName
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

func TestAnnotateHintsMarksTopSimilarTags(t *testing.T) {
	store := &catalogTestStore{
		tags: []issues.Tag{
			{Name: "database", Embedding: []float64{0.9, 0.1, 0.0}},
			{Name: "cleanup", Embedding: []float64{0.1, 0.1, 0.8}},
			{Name: "suggested-database-migration", Embedding: []float64{0.85, 0.15, 0.0}},
			{Name: "performance", Embedding: []float64{0.0, 0.9, 0.1}},
			{Name: "backend", Embedding: []float64{0.5, 0.5, 0.0}},
		},
	}
	service := NewCatalogService(store, nil, slog.Default())

	taxonomy := []ai.Tag{
		{Name: "database"},
		{Name: "cleanup"},
		{Name: "suggested-database-migration"},
		{Name: "performance"},
		{Name: "backend"},
	}

	// Issue embedding is close to database/migration, far from cleanup/performance.
	issueEmbedding := []float64{0.9, 0.1, 0.0}
	result := service.AnnotateHints(context.Background(), taxonomy, issueEmbedding, 2)

	if len(result) != len(taxonomy) {
		t.Fatalf("expected %d tags, got %d", len(taxonomy), len(result))
	}

	hintCount := 0
	hintNames := make(map[string]bool)
	for _, tag := range result {
		if tag.Hint {
			hintCount++
			hintNames[tag.Name] = true
		}
	}

	if hintCount != 2 {
		t.Fatalf("expected 2 hint tags, got %d", hintCount)
	}
	if !hintNames["database"] {
		t.Fatal("expected 'database' to be a hint (high similarity)")
	}
	if !hintNames["suggested-database-migration"] {
		t.Fatal("expected 'suggested-database-migration' to be a hint (high similarity)")
	}
}

func TestAnnotateHintsNoOpWithEmptyEmbedding(t *testing.T) {
	store := &catalogTestStore{
		tags: []issues.Tag{
			{Name: "database", Embedding: []float64{0.9, 0.1, 0.0}},
		},
	}
	service := NewCatalogService(store, nil, slog.Default())

	taxonomy := []ai.Tag{{Name: "database"}}
	result := service.AnnotateHints(context.Background(), taxonomy, nil, 5)

	for _, tag := range result {
		if tag.Hint {
			t.Fatalf("expected no hints with empty embedding, got hint on %q", tag.Name)
		}
	}
}

func TestAnnotateHintsDoesNotMutateOriginal(t *testing.T) {
	store := &catalogTestStore{
		tags: []issues.Tag{
			{Name: "database", Embedding: []float64{0.9, 0.1, 0.0}},
		},
	}
	service := NewCatalogService(store, nil, slog.Default())

	taxonomy := []ai.Tag{{Name: "database"}}
	_ = service.AnnotateHints(context.Background(), taxonomy, []float64{0.9, 0.1, 0.0}, 5)

	if taxonomy[0].Hint {
		t.Fatal("AnnotateHints mutated the original taxonomy slice")
	}
}

func TestIssueTaxonomyShortlistPrefersNearestTagsAndKeepsAnchors(t *testing.T) {
	store := &catalogTestStore{
		tags: []issues.Tag{
			{Name: "bug", Description: "software defect", Embedding: []float64{0.2, 0.2, 0}},
			{Name: "feature", Description: "new capability", Embedding: []float64{0.1, 0.1, 0}},
			{Name: "search", Description: "query and filtering", Embedding: []float64{1, 0, 0}},
			{Name: "autocomplete", Description: "typeahead search suggestions", Embedding: []float64{0.95, 0.05, 0}},
			{Name: "billing", Description: "payments", Embedding: []float64{0, 1, 0}},
		},
	}
	service := NewCatalogService(store, nil, slog.Default())

	result, err := service.IssueTaxonomyShortlist(context.Background(), []float64{1, 0, 0}, nil, 2)
	if err != nil {
		t.Fatalf("IssueTaxonomyShortlist: %v", err)
	}

	names := make([]string, 0, len(result))
	for _, tag := range result {
		names = append(names, tag.Name)
	}

	for _, want := range []string{"search", "autocomplete", "bug", "feature"} {
		if !slices.Contains(names, want) {
			t.Fatalf("expected shortlist to contain %q, got %#v", want, names)
		}
	}
	if slices.Contains(names, "billing") {
		t.Fatalf("expected shortlist to exclude distant tag billing, got %#v", names)
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
