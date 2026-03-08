package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"splat/internal/ai"
	"splat/internal/issues"
	issuemap "splat/internal/map"
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
	return "fake"
}

func (t *fakeTagger) Model() string {
	return "tagger-test"
}

type fakeEmbedder struct {
	result ai.EmbeddingResult
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/debug/issues/analyze", body)
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
	if len(tagger.capturedTags) != len(issuemap.AllTags()) {
		t.Fatalf("expected %d default tags, got %d", len(issuemap.AllTags()), len(tagger.capturedTags))
	}
	if tagger.capturedTags[0].Description == "" {
		t.Fatalf("expected default tags to include descriptions")
	}
}

func TestDebugIssueAnalyzeEndpointReturnsEmbeddingSimilarities(t *testing.T) {
	store := newSQLiteIssueStore(t, nil)
	if err := store.Replace(context.Background(), []issues.Issue{
		{
			ID:        "issue-1",
			Raw:       "export fails in safari",
			Tags:      []string{"export", "safari"},
			CreatedBy: "Casey",
			CreatedAt: issues.SeedIssues()[0].CreatedAt,
			Embedding: []float64{1, 0},
		},
		{
			ID:        "issue-2",
			Raw:       "search is slow",
			Tags:      []string{"search", "performance"},
			CreatedBy: "Casey",
			CreatedAt: issues.SeedIssues()[1].CreatedAt,
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

	req := httptest.NewRequest(http.MethodPost, "/api/debug/issues/analyze", bytes.NewBufferString(`{"text":"export issue"}`))
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
	req := httptest.NewRequest(http.MethodPost, "/api/debug/issues/analyze", body)
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

func TestDebugIssueAnalyzeEndpointRejectsInvalidInput(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
	})
	handler := server.Handler()

	t.Run("wrong method", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/debug/issues/analyze", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPost,
			"/api/debug/issues/analyze",
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
			"/api/debug/issues/analyze",
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
			"/api/debug/issues/analyze",
			bytes.NewBufferString(`{"text":"example"}`),
		)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", rec.Code)
		}
	})
}

func TestDebugIssueSampleLoadAndResetEndpoints(t *testing.T) {
	store := newSQLiteIssueStore(t, nil)
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  store,
	})
	handler := server.Handler()

	sampleReq := httptest.NewRequest(http.MethodPost, "/api/debug/issues/sample", nil)
	sampleRec := httptest.NewRecorder()
	handler.ServeHTTP(sampleRec, sampleReq)

	if sampleRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from sample load, got %d", sampleRec.Code)
	}

	var samplePayload debugIssueStoreResponse
	if err := json.NewDecoder(sampleRec.Body).Decode(&samplePayload); err != nil {
		t.Fatalf("failed to decode sample load response: %v", err)
	}
	if samplePayload.IssueCount != len(issues.SeedIssues()) {
		t.Fatalf("expected %d sample issues, got %d", len(issues.SeedIssues()), samplePayload.IssueCount)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/issues", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)

	var listPayload issuesResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listPayload); err != nil {
		t.Fatalf("failed to decode issue list response: %v", err)
	}
	if len(listPayload.Issues) != len(issues.SeedIssues()) {
		t.Fatalf("expected %d loaded issues, got %d", len(issues.SeedIssues()), len(listPayload.Issues))
	}

	stored, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("failed to list stored issues: %v", err)
	}
	if len(stored) == 0 {
		t.Fatal("expected sample issues in store")
	}
	if len(stored[0].TagScores) == 0 {
		t.Fatal("expected sampled issue to include analyzed tag scores")
	}
	if len(stored[0].Embedding) == 0 {
		t.Fatal("expected sampled issue to include embedding vector")
	}

	resetReq := httptest.NewRequest(http.MethodPost, "/api/debug/issues/reset", nil)
	resetRec := httptest.NewRecorder()
	handler.ServeHTTP(resetRec, resetReq)

	if resetRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from reset, got %d", resetRec.Code)
	}

	var resetPayload debugIssueStoreResponse
	if err := json.NewDecoder(resetRec.Body).Decode(&resetPayload); err != nil {
		t.Fatalf("failed to decode reset response: %v", err)
	}
	if resetPayload.IssueCount != 0 {
		t.Fatalf("expected 0 issues after reset, got %d", resetPayload.IssueCount)
	}
}
