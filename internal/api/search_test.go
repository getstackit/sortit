package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"splat/internal/ai"
	"splat/internal/issues"
	"splat/internal/queries"
)

func TestUnifiedSearchEndpointReturnsIssuesAndRelatedTags(t *testing.T) {
	store := newPostgresIssueStore(t, nil)
	if err := store.Replace(context.Background(), []issues.Issue{
		{
			ID:        "issue-export-target",
			Raw:       "Safari export crashes on PDF download",
			CreatedBy: "Casey",
			CreatedAt: issues.FixtureIssues()[0].CreatedAt,
			Status:    issues.StatusOpen,
			TagScores: []issues.TagRelevance{
				{Tag: "export", Relevance: 0.95},
				{Tag: "safari", Relevance: 0.92},
				{Tag: "bug", Relevance: 0.9},
			},
			Embedding: []float64{1, 0, 0},
		},
		{
			ID:        "issue-export-related",
			Raw:       "PDF export fails in Safari after tapping share twice",
			CreatedBy: "Jordan",
			CreatedAt: issues.FixtureIssues()[1].CreatedAt,
			Status:    issues.StatusOpen,
			TagScores: []issues.TagRelevance{
				{Tag: "export", Relevance: 0.9},
				{Tag: "safari", Relevance: 0.84},
			},
			Embedding: []float64{0.94, 0.06, 0},
		},
		{
			ID:        "issue-unrelated",
			Raw:       "Backfill analytics warehouse tables",
			CreatedBy: "Casey",
			CreatedAt: issues.FixtureIssues()[2].CreatedAt,
			Status:    issues.StatusOpen,
			TagScores: []issues.TagRelevance{
				{Tag: "backend", Relevance: 0.95},
			},
			Embedding: []float64{0, 1, 0},
		},
	}); err != nil {
		t.Fatalf("seed issues: %v", err)
	}
	if err := store.UpsertTags(context.Background(), []issues.Tag{
		{Name: "export", Embedding: []float64{1, 0, 0}},
		{Name: "safari", Embedding: []float64{0.9, 0.1, 0}},
		{Name: "bug", Embedding: []float64{0.8, 0.2, 0}},
	}); err != nil {
		t.Fatalf("seed tags: %v", err)
	}

	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		Analyzer: ai.NewAnalyzer(&fakeTagger{
			scores: []ai.TagScore{
				{Tag: "export", Relevance: 0.97},
				{Tag: "safari", Relevance: 0.91},
			},
		}, &fakeEmbedder{
			result: ai.EmbeddingResult{
				Vector: []float32{1, 0, 0},
				Info: ai.EmbeddingInfo{
					Dimensions:          3,
					Preview:             []float32{1, 0, 0},
					ChunkCount:          1,
					EstimatedTokenCount: 3,
					PooledFromChunks:    false,
				},
			},
		}),
		IssueStore: store,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/search?q=safari%20pdf%20export&limit=2", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for unified search, got %d", rec.Code)
	}

	var payload queries.SearchUnifiedResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode unified search response: %v", err)
	}

	if payload.Query.Raw != "safari pdf export" {
		t.Fatalf("expected query echo, got %q", payload.Query.Raw)
	}
	if len(payload.Issues) != 2 {
		t.Fatalf("expected 2 unified search issues, got %#v", payload.Issues)
	}
	issueIDs := []string{payload.Issues[0].ID, payload.Issues[1].ID}
	sort.Strings(issueIDs)
	if want := []string{"issue-export-related", "issue-export-target"}; !equalStrings(issueIDs, want) {
		t.Fatalf("expected export issues in unified results, got %v", issueIDs)
	}
	if len(payload.RelatedTags) == 0 || payload.RelatedTags[0].Name != "export" {
		t.Fatalf("expected export related tag first, got %#v", payload.RelatedTags)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
