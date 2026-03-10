package queries

import (
	"context"
	"encoding/json"
	"testing"

	"splat/internal/issues"
	issuemap "splat/internal/map"
	"splat/internal/services"
)

type memoryCorpusProjectionStore struct {
	payloads map[uint64][]byte
}

func (m *memoryCorpusProjectionStore) GetDerivedCorpusProjection(_ context.Context, revision uint64) ([]byte, error) {
	if payload, ok := m.payloads[revision]; ok {
		return payload, nil
	}
	return nil, issues.ErrDerivedCorpusNotFound
}

func (m *memoryCorpusProjectionStore) SaveDerivedCorpusProjection(_ context.Context, revision uint64, payload []byte) error {
	if m.payloads == nil {
		m.payloads = make(map[uint64][]byte)
	}
	m.payloads[revision] = append([]byte(nil), payload...)
	return nil
}

func TestDerivedCorpusLoaderPersistsAndReloadsProjection(t *testing.T) {
	ctx := context.Background()
	store := issues.NewInMemoryStore(issues.FixtureIssues())
	catalog := services.NewCatalogService(nil, services.FallbackAnalyzer(nil))
	projections := &memoryCorpusProjectionStore{}
	revisions := issues.NewRevisionTracker()

	loader := &DerivedCorpusLoader{
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

	var decoded issuemap.DerivedCorpus
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
