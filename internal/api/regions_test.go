package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/regions"
)

func newRegionsTestServer(t *testing.T) *Server {
	t.Helper()
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	store := issues.NewInMemoryStore([]issues.Issue{
		{
			ID:        "issue-1",
			Raw:       "auth fails",
			CreatedAt: now.Add(-3 * 24 * time.Hour),
			Status:    issues.StatusOpen,
			TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}},
		},
		{
			ID:        "issue-2",
			Raw:       "token refresh",
			CreatedAt: now.Add(-40 * 24 * time.Hour),
			Status:    issues.StatusOpen,
			TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.6}},
		},
		{
			ID:        "issue-3",
			Raw:       "billing question",
			CreatedAt: now.Add(-100 * 24 * time.Hour),
			Status:    issues.StatusClosed,
			TagScores: []domain.TagRelevance{{Tag: "billing", Relevance: 0.7}},
		},
	})
	return NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api/v1"},
		IssueStore:  store,
	})
}

func TestRegionsListReturnsTagRegions(t *testing.T) {
	server := newRegionsTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/ui/regions?kind=tag&window=30d", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload regions.ListResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Window.Label != "30d" {
		t.Fatalf("expected window 30d, got %q", payload.Window.Label)
	}
	if len(payload.Regions) != 2 {
		t.Fatalf("expected 2 regions (auth, billing), got %d", len(payload.Regions))
	}
	if payload.Regions[0].Region.Key.ID != "auth" {
		t.Fatalf("expected auth first (mass=2), got %q", payload.Regions[0].Region.Key.ID)
	}
	if payload.Regions[0].Metrics.Mass != 2 || payload.Regions[0].Metrics.MassOpen != 2 {
		t.Fatalf("auth metrics unexpected: %+v", payload.Regions[0].Metrics)
	}
}

func TestRegionsListDefaultsToTagKindAnd30Day(t *testing.T) {
	server := newRegionsTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/ui/regions", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload regions.ListResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Window.Label != "30d" {
		t.Fatalf("expected default window 30d, got %q", payload.Window.Label)
	}
}

func TestRegionsListRejectsUnsupportedKind(t *testing.T) {
	server := newRegionsTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/ui/regions?kind=custom", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRegionsListAcceptsClusterKind(t *testing.T) {
	server := newRegionsTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/ui/regions?kind=cluster&window=30d", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for cluster kind, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRegionGetReturnsRegion(t *testing.T) {
	server := newRegionsTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/ui/regions/tag/auth?window=30d", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload regions.GetResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Region.Key.ID != "auth" {
		t.Fatalf("expected auth, got %q", payload.Region.Key.ID)
	}
	if payload.Metrics.Mass != 2 {
		t.Fatalf("expected mass 2, got %d", payload.Metrics.Mass)
	}
}

func TestRegionGetReturnsNotFound(t *testing.T) {
	server := newRegionsTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/ui/regions/tag/ghost", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRegionGetRejectsCustomKind(t *testing.T) {
	server := newRegionsTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/ui/regions/custom/anything", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRegionGetClusterNotFoundWhenNoProjection(t *testing.T) {
	server := newRegionsTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/ui/regions/cluster/c-deadbeef", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown cluster, got %d: %s", rec.Code, rec.Body.String())
	}
}
