package mapview

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"testing"

	"sortit/internal/domain"
	"sortit/internal/issues"
	issuemap "sortit/internal/map"
	"sortit/internal/tags"
)

type memoryMapProjectionStore struct {
	payloads map[uint64][]byte
}

type stubMapProjectionStore struct {
	items     []issues.MapProjectionIssue
	tags      []issues.Tag
	loadCalls int
	listCalls int
	getCalls  int
}

func (s *stubMapProjectionStore) LoadMapProjectionData(_ context.Context) ([]issues.MapProjectionIssue, []issues.Tag, error) {
	s.loadCalls++
	return append([]issues.MapProjectionIssue(nil), s.items...), append([]issues.Tag(nil), s.tags...), nil
}

func (s *stubMapProjectionStore) List(_ context.Context) ([]issues.Issue, error) {
	s.listCalls++
	items := make([]issues.Issue, 0, len(s.items))
	for _, item := range s.items {
		items = append(items, issues.Issue{
			ID:        item.ID,
			Raw:       item.Raw,
			Tags:      append([]string(nil), item.Tags...),
			Status:    item.Status,
			TagScores: append([]domain.TagRelevance(nil), item.TagScores...),
			Embedding: append([]float64(nil), item.Embedding...),
			Links:     append([]issues.IssueLink(nil), item.Links...),
		})
	}
	return items, nil
}

func (s *stubMapProjectionStore) Get(_ context.Context, id string) (issues.Issue, error) {
	return s.GetIssueDetail(context.Background(), id)
}

func (s *stubMapProjectionStore) GetIssueDetail(_ context.Context, id string) (issues.Issue, error) {
	s.getCalls++
	for _, item := range s.items {
		if item.ID == id {
			return issues.Issue{
				ID:        item.ID,
				Raw:       item.Raw,
				Tags:      append([]string(nil), item.Tags...),
				Status:    item.Status,
				TagScores: append([]domain.TagRelevance(nil), item.TagScores...),
				Embedding: append([]float64(nil), item.Embedding...),
				Links:     append([]issues.IssueLink(nil), item.Links...),
			}, nil
		}
	}
	return issues.Issue{}, issues.ErrNotFound
}

func (s *stubMapProjectionStore) SaveIssue(context.Context, issues.Issue) error { return nil }

func (s *stubMapProjectionStore) SaveIssuePost(context.Context, issues.IssuePost) error {
	return nil
}

func (s *stubMapProjectionStore) UpdateIssueFields(context.Context, string, issues.IssueFieldUpdate) error {
	return nil
}

func (s *stubMapProjectionStore) SaveOperation(context.Context, issues.IssueOperation) error {
	return nil
}

func (s *stubMapProjectionStore) SaveLink(context.Context, issues.IssueLink) error { return nil }

func (m *memoryMapProjectionStore) GetMapProjection(_ context.Context, revision uint64) ([]byte, error) {
	if payload, ok := m.payloads[revision]; ok {
		return payload, nil
	}
	return nil, issues.ErrMapProjectionNotFound
}

func (m *memoryMapProjectionStore) SaveMapProjection(_ context.Context, revision uint64, payload []byte) error {
	if m.payloads == nil {
		m.payloads = make(map[uint64][]byte)
	}
	m.payloads[revision] = append([]byte(nil), payload...)
	return nil
}

func TestMapProjectionLoaderPersistsAndReloadsProjection(t *testing.T) {
	ctx := context.Background()
	store := issues.NewInMemoryStore(issues.FixtureIssues())
	catalog := tags.NewCatalogService(nil, tags.FallbackAnalyzer(nil), slog.Default())
	projections := &memoryMapProjectionStore{}
	revisions := issues.NewRevisionTracker()

	loader := &MapProjectionLoader{
		Store:       store,
		Catalog:     catalog,
		Revisions:   revisions,
		Projections: projections,
	}

	first, err := loader.Current(ctx)
	if err != nil {
		t.Fatalf("first current: %v", err)
	}
	if len(projections.payloads) != 1 {
		t.Fatalf("expected one persisted projection, got %d", len(projections.payloads))
	}

	payload, ok := projections.payloads[revisions.Revision()]
	if !ok {
		t.Fatalf("expected persisted payload for revision %d", revisions.Revision())
	}

	var decoded issuemap.MapProjection
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode persisted payload: %v", err)
	}
	if len(decoded.MapIssues) != len(first.MapIssues) {
		t.Fatalf("expected persisted corpus to match map issue count, got %d want %d", len(decoded.MapIssues), len(first.MapIssues))
	}

	projections.payloads[revisions.Revision()] = payload
	second, err := loader.Current(ctx)
	if err != nil {
		t.Fatalf("second current: %v", err)
	}
	if len(second.MapIssues) != len(first.MapIssues) {
		t.Fatalf("expected reloaded corpus to match original issue count, got %d want %d", len(second.MapIssues), len(first.MapIssues))
	}
}

func TestMapProjectionLoaderAlignsRebuildsToPreviousLayout(t *testing.T) {
	ctx := context.Background()
	store := issues.NewInMemoryStore(issues.FixtureIssues())
	catalog := tags.NewCatalogService(nil, tags.FallbackAnalyzer(nil), slog.Default())

	// No Projections persistence: every Current call rebuilds, exercising
	// the in-memory previous-layout cache on its own.
	loader := &MapProjectionLoader{
		Store:     store,
		Catalog:   catalog,
		Revisions: issues.NewRevisionTracker(),
	}

	first, err := loader.Current(ctx)
	if err != nil {
		t.Fatalf("first current: %v", err)
	}
	if len(first.MapIssues) == 0 {
		t.Fatal("expected map issues in first projection")
	}
	if len(loader.previousLayout()) != len(first.MapIssues) {
		t.Fatalf("expected previous layout cache to hold %d positions, got %d", len(first.MapIssues), len(loader.previousLayout()))
	}

	// Replace the remembered layout with a mirror image. If the cache is
	// threaded into the rebuild, Procrustes alignment must reproduce the
	// mirrored orientation instead of the data-driven one.
	mirrored := make(map[string]issuemap.Position, len(first.MapIssues))
	for _, item := range first.MapIssues {
		mirrored[item.ID] = issuemap.Position{X: 1 - item.X, Y: item.Y}
	}
	loader.prevMu.Lock()
	loader.prevPositions = mirrored
	loader.prevMu.Unlock()

	second, err := loader.Current(ctx)
	if err != nil {
		t.Fatalf("second current: %v", err)
	}

	const epsilon = 0.05
	for _, item := range second.MapIssues {
		want := mirrored[item.ID]
		if math.Abs(item.X-want.X) > epsilon || math.Abs(item.Y-want.Y) > epsilon {
			t.Errorf("rebuild ignored previous layout for %s: want near (%v, %v), got (%v, %v)", item.ID, want.X, want.Y, item.X, item.Y)
		}
	}
}

func TestMapProjectionLoaderUsesBulkProjectionStoreWhenAvailable(t *testing.T) {
	ctx := context.Background()
	store := &stubMapProjectionStore{
		items: issues.MapProjectionIssuesFromIssues(issues.FixtureIssues()),
		tags: []issues.Tag{
			{Name: "search"},
		},
	}

	loader := &MapProjectionLoader{
		Store:     store,
		Catalog:   tags.NewCatalogService(nil, tags.FallbackAnalyzer(nil), slog.Default()),
		Revisions: issues.NewRevisionTracker(),
	}

	corpus, err := loader.Current(ctx)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if len(corpus.MapIssues) == 0 {
		t.Fatal("expected map projection issues")
	}
	if store.loadCalls != 1 {
		t.Fatalf("expected one bulk projection load, got %d", store.loadCalls)
	}
	if store.listCalls != 0 {
		t.Fatalf("expected no list fallback calls, got %d", store.listCalls)
	}
	if store.getCalls != 0 {
		t.Fatalf("expected no get fallback calls, got %d", store.getCalls)
	}
}
