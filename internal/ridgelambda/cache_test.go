package ridgelambda

import (
	"context"
	"math"
	"testing"

	"sortit/internal/centering"
	"sortit/internal/domain"
	"sortit/internal/issuemath"
	"sortit/internal/issues"
	"sortit/internal/scoring"
)

type stubStore struct {
	items []issues.Issue
	calls int
}

func (s *stubStore) List(context.Context) ([]issues.Issue, error) {
	s.calls++
	return s.items, nil
}

type stubTags struct {
	tags []issues.Tag
}

func (s *stubTags) StoredTags(context.Context) ([]issues.Tag, error) {
	return s.tags, nil
}

type stubRevisions struct {
	rev uint64
}

func (s *stubRevisions) Revision() uint64 { return s.rev }

func unit(v []float64) []float64 {
	var mag float64
	for _, x := range v {
		mag += x * x
	}
	if mag == 0 {
		return v
	}
	mag = math.Sqrt(mag)
	out := make([]float64, len(v))
	for i := range v {
		out[i] = v[i] / mag
	}
	return out
}

// twoClusterCorpus is large enough to decompose and has separable tag
// structure, so GCV returns a usable penalty.
func twoClusterCorpus() ([]issues.Issue, []issues.Tag) {
	tags := []issues.Tag{
		{Name: "alpha", Embedding: unit([]float64{1, 0, 0, 0})},
		{Name: "beta", Embedding: unit([]float64{0, 1, 0, 0})},
	}
	items := []issues.Issue{
		{ID: "a1", TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.9}}, Embedding: unit([]float64{0.9, 0.2, 0.1, 0})},
		{ID: "a2", TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.8}}, Embedding: unit([]float64{0.95, 0.1, 0.2, 0})},
		{ID: "a3", TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.85}}, Embedding: unit([]float64{0.92, 0.15, 0.1, 0})},
		{ID: "b1", TagScores: []domain.TagRelevance{{Tag: "beta", Relevance: 0.9}}, Embedding: unit([]float64{0.1, 0.9, 0.2, 0})},
		{ID: "b2", TagScores: []domain.TagRelevance{{Tag: "beta", Relevance: 0.8}}, Embedding: unit([]float64{0.2, 0.95, 0.1, 0})},
		{ID: "b3", TagScores: []domain.TagRelevance{{Tag: "beta", Relevance: 0.85}}, Embedding: unit([]float64{0.15, 0.92, 0.1, 0})},
	}
	return items, tags
}

func TestCacheNilSafe(t *testing.T) {
	var c *Cache
	lambda, ok, err := c.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if ok || lambda != 0 {
		t.Fatalf("nil cache should yield (0, false), got (%v, %v)", lambda, ok)
	}
}

func TestCacheSelectsAndMemoizesByRevision(t *testing.T) {
	items, tags := twoClusterCorpus()
	store := &stubStore{items: items}
	rev := &stubRevisions{rev: 1}
	c := &Cache{Store: store, Tags: &stubTags{tags: tags}, Revisions: rev}

	lambda, ok, err := c.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if !ok || lambda <= 0 {
		t.Fatalf("expected a positive selected penalty, got (%v, %v)", lambda, ok)
	}
	firstCalls := store.calls

	// Same revision → memoized, no recompute.
	if _, _, err := c.Current(context.Background()); err != nil {
		t.Fatalf("Current (cached): %v", err)
	}
	if store.calls != firstCalls {
		t.Fatalf("expected memoized result at the same revision, store called %d→%d", firstCalls, store.calls)
	}

	// Bump revision → recompute.
	rev.rev = 2
	if _, _, err := c.Current(context.Background()); err != nil {
		t.Fatalf("Current (new revision): %v", err)
	}
	if store.calls <= firstCalls {
		t.Fatalf("expected recompute after revision bump, store calls stayed at %d", store.calls)
	}
}

func TestCacheRejectsSmallCorpus(t *testing.T) {
	items, tags := twoClusterCorpus()
	store := &stubStore{items: items[:3]} // below MinDecompositionIssues
	c := &Cache{Store: store, Tags: &stubTags{tags: tags}}

	lambda, ok, err := c.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if ok || lambda != 0 {
		t.Fatalf("small corpus should yield (0, false), got (%v, %v)", lambda, ok)
	}
}

func TestStrideSampleIsDeterministicAndBounded(t *testing.T) {
	items := make([]issues.Issue, 100)
	for i := range items {
		items[i] = issues.Issue{ID: string(rune('A'+i%26)) + string(rune('0'+i/26))}
	}
	got := strideSample(items, 10)
	if len(got) != 10 {
		t.Fatalf("expected 10 sampled issues, got %d", len(got))
	}
	again := strideSample(items, 10)
	for i := range got {
		if got[i].ID != again[i].ID {
			t.Fatalf("stride sample not deterministic at %d: %q vs %q", i, got[i].ID, again[i].ID)
		}
	}
	// Under the cap: returns everything.
	if n := len(strideSample(items[:5], 10)); n != 5 {
		t.Fatalf("expected all 5 issues when under cap, got %d", n)
	}
}

// TestCacheUncenteredSelection asserts the uncentered regime (WP-304): λ is
// selected on unit-normalized, uncentered inputs — exactly what
// SelectRidgeLambdaGCV returns for CenterEmbeddingsWith(zero means) — and a
// wired centering cache is ignored.
func TestCacheUncenteredSelection(t *testing.T) {
	items, tags := twoClusterCorpus()
	store := &stubStore{items: items}
	c := &Cache{
		Store:      store,
		Tags:       &stubTags{tags: tags},
		Uncentered: true,
		// A wired centering cache must be ignored in the uncentered regime.
		Centering: &centering.Cache{Store: &centeringStubStore{items: items}},
	}

	lambda, ok, err := c.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if !ok || lambda <= 0 {
		t.Fatalf("expected a positive uncentered penalty, got (%v, %v)", lambda, ok)
	}

	// Reference: the same selection on explicitly uncentered inputs.
	tagNames := []string{"alpha", "beta"}
	rawIssues := make(map[string][]float64, len(items))
	for _, item := range items {
		rawIssues[item.ID] = item.Embedding
	}
	rawTags := map[string][]float64{"alpha": tags[0].Embedding, "beta": tags[1].Embedding}
	issueEmb, tagEmb := issuemath.CenterEmbeddingsWith(issuemath.CorpusMeans{}, rawIssues, rawTags)
	want, wantOK := issuemath.SelectRidgeLambdaGCV(items, tagNames, issueEmb, tagEmb,
		scoring.RidgeAnchorLambdaScored, nil)
	if !wantOK {
		t.Fatal("reference GCV selection fell back")
	}
	if lambda != want {
		t.Fatalf("uncentered cache selected λ=%v, reference uncentered selection is %v", lambda, want)
	}
}

// centeringStubStore adapts the test corpus to the centering cache's lister.
type centeringStubStore struct {
	items []issues.Issue
}

func (s *centeringStubStore) List(context.Context) ([]issues.Issue, error) {
	return s.items, nil
}
