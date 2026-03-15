package queries

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"splat/internal/issues"
	issuemap "splat/internal/map"
	"splat/internal/services"
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
			TagScores: append([]issues.TagRelevance(nil), item.TagScores...),
			Embedding: append([]float64(nil), item.Embedding...),
			Links:     append([]issues.IssueLink(nil), item.Links...),
		})
	}
	return items, nil
}

func (s *stubMapProjectionStore) Get(_ context.Context, id string) (issues.Issue, error) {
	s.getCalls++
	for _, item := range s.items {
		if item.ID == id {
			return issues.Issue{
				ID:        item.ID,
				Raw:       item.Raw,
				Tags:      append([]string(nil), item.Tags...),
				Status:    item.Status,
				TagScores: append([]issues.TagRelevance(nil), item.TagScores...),
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
	catalog := services.NewCatalogService(nil, services.FallbackAnalyzer(nil), slog.Default())
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
		Catalog:   services.NewCatalogService(nil, services.FallbackAnalyzer(nil), slog.Default()),
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
