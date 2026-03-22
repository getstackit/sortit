package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"splat/internal/ai"
	"splat/internal/issues"
	"splat/internal/queries"
)

type fakeTagger struct {
	capturedTags []ai.Tag
	scores       []ai.TagScore
}

func (t *fakeTagger) Score(_ context.Context, _ string, tags []ai.Tag) ([]ai.TagScore, error) {
	t.capturedTags = append([]ai.Tag(nil), tags...)
	return append([]ai.TagScore(nil), t.scores...), nil
}

func (t *fakeTagger) Provider() string {
	return "fake" //nolint:goconst
}

func (t *fakeTagger) Model() string {
	return "tagger-test"
}

type fakeEmbedder struct {
	result ai.EmbeddingResult
}

type debugInvalidationStore struct {
	issues.Store
	calls int
	err   error
}

func (s *debugInvalidationStore) InvalidateMapProjections(context.Context) error {
	s.calls++
	return s.err
}

func (e *fakeEmbedder) EmbedText(_ context.Context, _ string) (ai.EmbeddingResult, error) {
	result := e.result
	result.Vector = append([]float32(nil), e.result.Vector...)
	result.Info.Preview = append([]float32(nil), e.result.Info.Preview...)
	return result, nil
}

func (e *fakeEmbedder) Provider() string {
	return "fake"
}

func (e *fakeEmbedder) Model() string {
	return "embedder-test"
}

func TestDebugIssueAnalyzeEndpoint(t *testing.T) {
	tagger := &fakeTagger{
		scores: []ai.TagScore{
			{Tag: "export", Relevance: 0.92},
			{Tag: "safari", Relevance: 0.88},
		},
	}
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api", "/api/v1"},
		Analyzer: ai.NewAnalyzer(tagger, &fakeEmbedder{
			result: ai.EmbeddingResult{
				Vector: []float32{0.5, -0.25, 0.75},
				Info: ai.EmbeddingInfo{
					Dimensions:          3,
					Preview:             []float32{0.5, -0.25, 0.75},
					ChunkCount:          1,
					EstimatedTokenCount: 9,
					PooledFromChunks:    false,
				},
			},
		}),
	})

	body := bytes.NewBufferString(`{"text":"Safari export crashes on iPad"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/ui/debug/issues/analyze", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload debugIssueAnalyzeResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if payload.Tagger.Provider != "fake" || payload.Tagger.Model != "tagger-test" {
		t.Fatalf("unexpected tagger metadata: %+v", payload.Tagger)
	}
	if payload.Embedder.Provider != "fake" || payload.Embedder.Model != "embedder-test" {
		t.Fatalf("unexpected embedder metadata: %+v", payload.Embedder)
	}
	if len(payload.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(payload.Tags))
	}
	if payload.Embedding.Dimensions != 3 {
		t.Fatalf("expected embedding dimensions 3, got %d", payload.Embedding.Dimensions)
	}
	if payload.Embedding.ChunkCount != 1 {
		t.Fatalf("expected 1 chunk, got %d", payload.Embedding.ChunkCount)
	}
	if len(payload.Embedding.Preview) != 3 {
		t.Fatalf("expected embedding preview length 3, got %d", len(payload.Embedding.Preview))
	}
	if payload.ComparedIssueCount != 0 {
		t.Fatalf("expected no compared issues by default, got %d", payload.ComparedIssueCount)
	}
	if len(tagger.capturedTags) != len(issues.DefaultTags()) {
		t.Fatalf("expected %d default tags, got %d", len(issues.DefaultTags()), len(tagger.capturedTags))
	}
	if tagger.capturedTags[0].Description == "" {
		t.Fatalf("expected default tags to include descriptions")
	}
}

func TestDebugIssueAnalyzeEndpointReturnsEmbeddingSimilarities(t *testing.T) {
	store := newPostgresIssueStore(t, nil)
	if err := store.Replace(context.Background(), []issues.Issue{
		{
			ID:        "issue-1",
			Raw:       "export fails in safari",
			Tags:      []string{"export", "safari"},
			CreatedBy: "Casey",
			CreatedAt: issues.FixtureIssues()[0].CreatedAt,
			Embedding: []float64{1, 0},
		},
		{
			ID:        "issue-2",
			Raw:       "search is slow",
			Tags:      []string{"search", "performance"},
			CreatedBy: "Casey",
			CreatedAt: issues.FixtureIssues()[1].CreatedAt,
			Embedding: []float64{0, 1},
		},
	}); err != nil {
		t.Fatalf("seed issues: %v", err)
	}

	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  store,
		Analyzer: ai.NewAnalyzer(&fakeTagger{}, &fakeEmbedder{
			result: ai.EmbeddingResult{
				Vector: []float32{1, 0},
				Info: ai.EmbeddingInfo{
					Dimensions:          2,
					Preview:             []float32{1, 0},
					ChunkCount:          1,
					EstimatedTokenCount: 4,
				},
			},
		}),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/ui/debug/issues/analyze", bytes.NewBufferString(`{"text":"export issue"}`))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload debugIssueAnalyzeResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	if payload.ComparedIssueCount != 2 {
		t.Fatalf("expected 2 compared issues, got %d", payload.ComparedIssueCount)
	}
	if payload.AverageIssueSimilarity != 0.5 {
		t.Fatalf("expected average similarity 0.5, got %v", payload.AverageIssueSimilarity)
	}
	if len(payload.SimilarIssues) != 2 {
		t.Fatalf("expected 2 similar issues, got %d", len(payload.SimilarIssues))
	}
	if payload.SimilarIssues[0].ID != "issue-1" || payload.SimilarIssues[0].Similarity != 1 {
		t.Fatalf("expected issue-1 similarity 1.0 first, got %+v", payload.SimilarIssues[0])
	}
}

func TestDebugIssueAnalyzeEndpointUsesCustomTags(t *testing.T) {
	tagger := &fakeTagger{
		scores: []ai.TagScore{{Tag: "custom", Relevance: 0.61}},
	}
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		Analyzer: ai.NewAnalyzer(tagger, &fakeEmbedder{
			result: ai.EmbeddingResult{
				Vector: []float32{1, 0},
				Info: ai.EmbeddingInfo{
					Dimensions:          2,
					Preview:             []float32{1, 0},
					ChunkCount:          1,
					EstimatedTokenCount: 3,
					PooledFromChunks:    false,
				},
			},
		}),
	})

	body := bytes.NewBufferString(`{"text":"Custom issue","tags":["custom","bug","custom"," "]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/ui/debug/issues/analyze", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(tagger.capturedTags) != 2 {
		t.Fatalf("expected 2 normalized custom tags, got %d", len(tagger.capturedTags))
	}
	if tagger.capturedTags[0].Name != "custom" || tagger.capturedTags[1].Name != "bug" {
		t.Fatalf("unexpected custom tags: %+v", tagger.capturedTags)
	}
}

func TestDebugEvalTagsEndpoint(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		Analyzer: ai.NewAnalyzer(&fakeTagger{}, &fakeEmbedder{
			result: ai.EmbeddingResult{
				Vector: []float32{1, 0},
				Info: ai.EmbeddingInfo{
					Dimensions:          2,
					Preview:             []float32{1, 0},
					ChunkCount:          1,
					EstimatedTokenCount: 4,
				},
			},
		}),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/ui/debug/eval-tags", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload queries.DebugEvalTagsResult
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Fixture != "corpus" {
		t.Fatalf("expected corpus fixture, got %q", payload.Fixture)
	}
	if payload.CaseCount == 0 {
		t.Fatal("expected at least one benchmark case")
	}
}

func TestDebugInvalidateMapProjectionEndpoint(t *testing.T) {
	store := &debugInvalidationStore{Store: issues.NewInMemoryStore(nil)}
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  store,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/ui/debug/map-projection/invalidate", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if store.calls != 1 {
		t.Fatalf("expected 1 invalidation call, got %d", store.calls)
	}

	var payload debugInvalidateMapProjectionResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if !payload.Invalidated {
		t.Fatal("expected invalidated=true")
	}
}

func TestDebugInvalidateMapProjectionEndpointReturnsNotImplementedWithoutInvalidator(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/ui/debug/map-projection/invalidate", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", rec.Code)
	}
}

func TestDebugInvalidateMapProjectionEndpointReturnsServerErrorOnInvalidationFailure(t *testing.T) {
	store := &debugInvalidationStore{
		Store: issues.NewInMemoryStore(nil),
		err:   errors.New("boom"),
	}
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  store,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/ui/debug/map-projection/invalidate", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestDebugIssueAnalyzeEndpointRejectsInvalidInput(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
	})
	handler := server.Handler()

	t.Run("wrong method", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/ui/debug/issues/analyze", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})

	t.Run("invalidate wrong method", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/ui/debug/map-projection/invalidate", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPost,
			"/api/ui/debug/issues/analyze",
			bytes.NewBufferString(`{"text":`),
		)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("missing text", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPost,
			"/api/ui/debug/issues/analyze",
			bytes.NewBufferString(`{"text":"   "}`),
		)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("analyzer not configured", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPost,
			"/api/ui/debug/issues/analyze",
			bytes.NewBufferString(`{"text":"example"}`),
		)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", rec.Code)
		}
	})
}
