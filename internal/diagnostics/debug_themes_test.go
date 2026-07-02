package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	"sortit/internal/issues"
	"sortit/internal/ridgedecomp"
	"sortit/internal/ridgelambda"
	"sortit/internal/tags"
	"sortit/internal/themes"
)

// debugThemesRevisions is a simple, single-threaded revision stub — the debug
// themes tests never race the revision, unlike internal/themes' own
// concurrency test.
type debugThemesRevisions struct{ rev uint64 }

func (r *debugThemesRevisions) Revision() uint64 { return r.rev }

// debugThemesTags is the two-tag catalog (orthogonal embeddings) the fixture
// corpus below anchors against.
func debugThemesTags() []issues.Tag {
	return []issues.Tag{
		{Name: "alpha", Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{Name: "beta", Embedding: unitVec64([]float64{0, 1, 0, 0})},
	}
}

// debugThemesIssues builds n anchored, embedded issues split between an
// alpha-leaning and a beta-leaning cluster, all satisfying the theme
// participation rule (usable embedding + anchored tag).
func debugThemesIssues(n int) []issues.Issue {
	items := make([]issues.Issue, 0, n)
	for i := range n {
		jitter := float64(i%5) * 0.01
		tag := "alpha"
		emb := unitVec64([]float64{0.9 + jitter, 0.15 + jitter, 0.1, 0})
		if i%2 == 1 {
			tag = "beta"
			emb = unitVec64([]float64{0.15 + jitter, 0.9 + jitter, 0.1, 0})
		}
		items = append(items, issues.Issue{
			ID:        fmt.Sprintf("issue-%03d", i),
			Raw:       fmt.Sprintf("issue %d about %s", i, tag),
			Status:    issues.StatusOpen,
			TagScores: []issues.TagRelevance{{Tag: tag, Relevance: 0.8 + jitter}},
			Embedding: emb,
		})
	}
	return items
}

// debugThemesFixture wires a themes.Cache and a ridgedecomp.Cache over the
// same store/catalog/revision, matching how internal/api/server.go wires
// production dependencies (both caches share one store and revision source).
type debugThemesFixture struct {
	store  *issues.InMemoryStore
	decomp *ridgedecomp.Cache
	themes *themes.Cache
	rev    *debugThemesRevisions
}

func newDebugThemesFixture(t *testing.T, items []issues.Issue) debugThemesFixture {
	t.Helper()
	ctx := context.Background()

	store := issues.NewInMemoryStore(items)
	tagStore := &debugTagStore{}
	catalog := tags.NewCatalogService(tagStore, nil, nil)
	if err := catalog.EnsureStoredTags(ctx, debugThemesTags()); err != nil {
		t.Fatalf("ensure stored tags: %v", err)
	}

	rev := &debugThemesRevisions{rev: 1}
	lambda := &ridgelambda.Cache{Store: store, Tags: catalog, Revisions: rev}
	decomp := &ridgedecomp.Cache{Store: store, Tags: catalog, Revisions: rev, Lambda: lambda}
	cache := &themes.Cache{Decomp: decomp, Store: store, Revisions: rev}

	return debugThemesFixture{store: store, decomp: decomp, themes: cache, rev: rev}
}

func TestDebugThemesListShapeAndOrder(t *testing.T) {
	ctx := context.Background()
	fx := newDebugThemesFixture(t, debugThemesIssues(24))

	result, err := (DebugThemesListHandler{Cache: fx.themes}).Handle(ctx)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if !result.Computed {
		t.Fatal("expected computed=true for a 24-issue corpus")
	}
	if result.Participating != 24 {
		t.Fatalf("expected 24 participating, got %d", result.Participating)
	}
	if len(result.Themes) == 0 {
		t.Fatal("expected at least one theme")
	}

	// Sorted by weight descending.
	for i := 1; i < len(result.Themes); i++ {
		if result.Themes[i].Weight > result.Themes[i-1].Weight {
			t.Fatalf("themes not sorted by weight desc: %+v", result.Themes)
		}
	}

	// Weight shares sum to ~1 across the list.
	total := 0.0
	for _, theme := range result.Themes {
		total += theme.Weight
		if theme.StableID == "" {
			t.Fatalf("theme missing stable id: %+v", theme)
		}
	}
	if math.Abs(total-1) > 0.01 {
		t.Fatalf("expected weight shares to sum to ~1, got %v", total)
	}

	// Every theme on a first-ever compute is minted (no predecessor).
	if len(result.MintedIDs) != len(result.Themes) {
		t.Fatalf("expected every theme minted on first compute, minted=%v themes=%v", result.MintedIDs, result.Themes)
	}
	for _, theme := range result.Themes {
		if !theme.Minted {
			t.Fatalf("expected theme %+v minted on first compute", theme)
		}
	}
	if len(result.RetiredIDs) != 0 {
		t.Fatalf("expected no retirements on first compute, got %v", result.RetiredIDs)
	}
}

func TestDebugThemesListDegradedWhenCacheNotOK(t *testing.T) {
	ctx := context.Background()
	// Below the theme participation floor (minThemeParticipants=16), so the
	// cache degrades to (zero, false).
	fx := newDebugThemesFixture(t, debugThemesIssues(6))

	result, err := (DebugThemesListHandler{Cache: fx.themes}).Handle(ctx)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Computed {
		t.Fatalf("expected computed=false for a below-floor corpus, got %+v", result)
	}
	if len(result.Themes) != 0 || result.Participating != 0 {
		t.Fatalf("expected a zero-value degraded response, got %+v", result)
	}
}

func TestDebugThemesListNilCacheDegrades(t *testing.T) {
	result, err := (DebugThemesListHandler{}).Handle(context.Background())
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Computed {
		t.Fatalf("expected computed=false with no cache wired, got %+v", result)
	}
}

func TestDebugThemeDetailReturnsCentroidAndWeightIssues(t *testing.T) {
	ctx := context.Background()
	fx := newDebugThemesFixture(t, debugThemesIssues(24))

	list, err := (DebugThemesListHandler{Cache: fx.themes}).Handle(ctx)
	if err != nil {
		t.Fatalf("list handle: %v", err)
	}
	if len(list.Themes) == 0 {
		t.Fatal("expected at least one theme from the list endpoint")
	}
	stableID := list.Themes[0].StableID

	detail, err := (DebugThemeDetailHandler{Cache: fx.themes, Store: fx.store}).Handle(ctx, stableID)
	if err != nil {
		t.Fatalf("detail handle: %v", err)
	}
	if !detail.Computed {
		t.Fatal("expected computed=true")
	}
	if detail.StableID != stableID {
		t.Fatalf("expected stable id %q, got %q", stableID, detail.StableID)
	}
	if detail.TagsNote == "" {
		t.Fatal("expected the top-5-tags limitation to be documented in TagsNote")
	}
	if len(detail.CentroidTopIssues) == 0 {
		t.Fatal("expected centroid-top issues to be populated")
	}
	if len(detail.CentroidTopIssues) > debugThemeTopIssues {
		t.Fatalf("expected at most %d centroid-top issues, got %d", debugThemeTopIssues, len(detail.CentroidTopIssues))
	}
	for i := 1; i < len(detail.CentroidTopIssues); i++ {
		if detail.CentroidTopIssues[i].Score > detail.CentroidTopIssues[i-1].Score {
			t.Fatalf("centroid-top issues not sorted by score desc: %+v", detail.CentroidTopIssues)
		}
	}
	if len(detail.TopIssuesByWeight) == 0 {
		t.Fatal("expected top-issues-by-weight to be populated")
	}
	if len(detail.TopIssuesByWeight) > debugThemeTopIssues {
		t.Fatalf("expected at most %d top-by-weight issues, got %d", debugThemeTopIssues, len(detail.TopIssuesByWeight))
	}
	for i := 1; i < len(detail.TopIssuesByWeight); i++ {
		if detail.TopIssuesByWeight[i].Score > detail.TopIssuesByWeight[i-1].Score {
			t.Fatalf("top-by-weight issues not sorted by score desc: %+v", detail.TopIssuesByWeight)
		}
	}
}

func TestDebugThemeDetailUnknownIDIs404(t *testing.T) {
	ctx := context.Background()
	fx := newDebugThemesFixture(t, debugThemesIssues(24))

	_, err := (DebugThemeDetailHandler{Cache: fx.themes, Store: fx.store}).Handle(ctx, "theme-does-not-exist")
	if !errors.Is(err, ErrThemeNotFound) {
		t.Fatalf("expected ErrThemeNotFound, got %v", err)
	}
}

func TestDebugThemeDetailDegradedWhenCacheNotOK(t *testing.T) {
	ctx := context.Background()
	fx := newDebugThemesFixture(t, debugThemesIssues(6))

	result, err := (DebugThemeDetailHandler{Cache: fx.themes, Store: fx.store}).Handle(ctx, "theme-1")
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Computed {
		t.Fatalf("expected computed=false for a below-floor corpus, got %+v", result)
	}
}

func TestDebugIssueThemesParticipating(t *testing.T) {
	ctx := context.Background()
	items := debugThemesIssues(24)
	fx := newDebugThemesFixture(t, items)
	targetID := items[0].ID

	result, err := (DebugIssueThemesHandler{Store: fx.store, Cache: fx.themes, RidgeDecomp: fx.decomp}).Handle(ctx, targetID)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if !result.Computed {
		t.Fatal("expected computed=true")
	}
	if result.Participation != string(themes.ParticipationParticipating) {
		t.Fatalf("expected participating, got %q", result.Participation)
	}
	if len(result.Themes) == 0 {
		t.Fatal("expected a non-empty theme distribution")
	}
	for i := 1; i < len(result.Themes); i++ {
		if result.Themes[i].Weight > result.Themes[i-1].Weight {
			t.Fatalf("issue themes not sorted by weight desc: %+v", result.Themes)
		}
	}
}

func TestDebugIssueThemesExcludedNoEmbedding(t *testing.T) {
	ctx := context.Background()
	items := debugThemesIssues(24)
	items = append(items, issues.Issue{
		ID:        "no-embed",
		Status:    issues.StatusOpen,
		TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 0.9}},
	})
	fx := newDebugThemesFixture(t, items)

	result, err := (DebugIssueThemesHandler{Store: fx.store, Cache: fx.themes, RidgeDecomp: fx.decomp}).Handle(ctx, "no-embed")
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Participation != string(themes.ParticipationExcludedNoEmbedding) {
		t.Fatalf("expected excluded-no-embedding, got %q", result.Participation)
	}
	if len(result.Themes) != 0 {
		t.Fatalf("expected no theme distribution for an excluded issue, got %+v", result.Themes)
	}
}

func TestDebugIssueThemesExcludedNoAnchor(t *testing.T) {
	ctx := context.Background()
	items := debugThemesIssues(24)
	items = append(items, issues.Issue{
		ID:        "no-anchor",
		Status:    issues.StatusOpen,
		Embedding: unitVec64([]float64{0.5, 0.5, 0.2, 0}),
	})
	fx := newDebugThemesFixture(t, items)

	result, err := (DebugIssueThemesHandler{Store: fx.store, Cache: fx.themes, RidgeDecomp: fx.decomp}).Handle(ctx, "no-anchor")
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Participation != string(themes.ParticipationExcludedNoAnchor) {
		t.Fatalf("expected excluded-no-anchor, got %q", result.Participation)
	}
	if len(result.Themes) != 0 {
		t.Fatalf("expected no theme distribution for an excluded issue, got %+v", result.Themes)
	}
}

func TestDebugIssueThemesUnknownIssueIs404(t *testing.T) {
	ctx := context.Background()
	fx := newDebugThemesFixture(t, debugThemesIssues(24))

	_, err := (DebugIssueThemesHandler{Store: fx.store, Cache: fx.themes, RidgeDecomp: fx.decomp}).Handle(ctx, "does-not-exist")
	if !errors.Is(err, issues.ErrNotFound) {
		t.Fatalf("expected issues.ErrNotFound, got %v", err)
	}
}

func TestDebugIssueThemesDegradedWhenCacheNotOK(t *testing.T) {
	ctx := context.Background()
	items := debugThemesIssues(6) // below the theme participation floor
	fx := newDebugThemesFixture(t, items)
	targetID := items[0].ID

	result, err := (DebugIssueThemesHandler{Store: fx.store, Cache: fx.themes, RidgeDecomp: fx.decomp}).Handle(ctx, targetID)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Computed {
		t.Fatalf("expected computed=false for a below-floor corpus, got %+v", result)
	}
	// The decomposition itself is still usable at 6 anchored+embedded issues
	// (well above scoring.MinDecompositionIssues=5), so participation is still
	// classifiable even though the theme cache has nothing to show.
	if result.Participation != string(themes.ParticipationParticipating) {
		t.Fatalf("expected participation still classifiable via the decomposition, got %q", result.Participation)
	}
}
