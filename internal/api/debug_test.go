package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sortit/internal/ai"
	"sortit/internal/diagnostics"
	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/tags"
)

type fakeTagger struct {
	capturedTags []ai.Tag
	scores       []ai.TagScore
}

func (t *fakeTagger) Score(_ context.Context, _ string, tags []ai.Tag, _ []ai.FewShotExample) ([]ai.TagScore, error) {
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

type debugAnalyzeStore struct {
	*issues.InMemoryStore
	tags []issues.Tag
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

func (s *debugAnalyzeStore) ListTags(context.Context) ([]issues.Tag, error) {
	out := make([]issues.Tag, len(s.tags))
	copy(out, s.tags)
	return out, nil
}

func (s *debugAnalyzeStore) UpsertTags(_ context.Context, tags []issues.Tag) error {
	out := make([]issues.Tag, len(tags))
	copy(out, tags)
	s.tags = out
	return nil
}

func (s *debugAnalyzeStore) UpdateTagSpecificity(context.Context, string, *float64, *float64, *float64, *time.Time) error {
	return nil
}

func TestDebugIssueAnalyzeEndpoint(t *testing.T) {
	tagger := &fakeTagger{
		scores: []ai.TagScore{
			{Tag: "export", Relevance: 0.92, Evidence: []string{"export crashes on iPad"}},
			{Tag: "safari", Relevance: 0.88, Evidence: []string{"Safari export crashes"}},
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
	if len(tagger.capturedTags) == 0 {
		t.Fatal("expected default taxonomy to be used when no stored tags exist")
	}
	if payload.CandidateSet.Mode != tags.CandidateModeRetrievalShortlist {
		t.Fatalf("expected retrieval-shortlist default mode, got %q", payload.CandidateSet.Mode)
	}
	if len(payload.CandidateSet.Tags) == 0 {
		t.Fatal("expected candidate set metadata in analyze response")
	}
	for _, tag := range payload.Tags {
		if tag.VerificationVerdict != domain.TagVerificationVerdictKeep {
			t.Fatalf("expected verifier to annotate kept tags by default, got %#v", payload.Tags)
		}
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

func TestDebugIssueAnalyzeEndpointCanDisableVerifier(t *testing.T) {
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
				},
			},
		}),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/ui/debug/issues/analyze", bytes.NewBufferString(`{"text":"Custom issue","verify":false}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload debugIssueAnalyzeResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	for _, tag := range payload.Tags {
		if tag.VerificationVerdict != "" {
			t.Fatalf("expected verifier metadata to be omitted when disabled, got %#v", payload.Tags)
		}
	}
}

func TestDebugIssueAnalyzeEndpointReturnsCandidateProvenance(t *testing.T) {
	store := &debugAnalyzeStore{InMemoryStore: issues.NewInMemoryStore(nil)}
	if err := store.UpsertTags(context.Background(), []issues.Tag{
		{Name: "backend", Description: "server-side logic", Embedding: []float64{0, 0.2}},
		{Name: "search", Description: "query and filtering", Embedding: []float64{1, 0}},
		{Name: "autocomplete", Description: "typeahead", Embedding: []float64{0.95, 0.05}},
		{Name: "billing", Description: "payments", Embedding: []float64{0, 1}},
	}); err != nil {
		t.Fatalf("upsert tags: %v", err)
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

	req := httptest.NewRequest(http.MethodPost, "/api/ui/debug/issues/analyze", bytes.NewBufferString(`{"text":"search box issue"}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload debugIssueAnalyzeResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.CandidateSet.Mode != tags.CandidateModeRetrievalShortlist {
		t.Fatalf("expected retrieval-shortlist candidate mode, got %q", payload.CandidateSet.Mode)
	}
	candidateByName := make(map[string]tags.CandidateTag, len(payload.CandidateSet.Tags))
	for _, tag := range payload.CandidateSet.Tags {
		candidateByName[tag.Name] = tag
	}
	search, ok := candidateByName["search"]
	if !ok {
		t.Fatalf("expected search in candidate set, got %#v", payload.CandidateSet.Tags)
	}
	if !containsCandidateSource(search.Sources, tags.CandidateSourceRetrieval) || !containsCandidateSource(search.Sources, tags.CandidateSourceAnchor) {
		t.Fatalf("expected search retrieval+anchor provenance, got %#v", search.Sources)
	}
	if _, ok := candidateByName["backend"]; !ok {
		t.Fatalf("expected backend anchor candidate, got %#v", payload.CandidateSet.Tags)
	}
	if _, ok := candidateByName["billing"]; ok {
		t.Fatalf("expected billing to be excluded from shortlist, got %#v", payload.CandidateSet.Tags)
	}
	if len(payload.Tags) != 0 {
		for _, tag := range payload.Tags {
			if len(tag.CandidateSources) == 0 {
				t.Fatalf("expected candidate source metadata on analyzed tags, got %#v", payload.Tags)
			}
		}
	}
}

func TestDebugIssueAnalyzeEndpointSupportsFullCatalogMode(t *testing.T) {
	store := &debugAnalyzeStore{InMemoryStore: issues.NewInMemoryStore(nil)}
	if err := store.UpsertTags(context.Background(), []issues.Tag{
		{Name: "bug", Description: "software defect"},
		{Name: "search", Description: "query and filtering"},
	}); err != nil {
		t.Fatalf("upsert tags: %v", err)
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

	req := httptest.NewRequest(http.MethodPost, "/api/ui/debug/issues/analyze", bytes.NewBufferString(`{"text":"search bug","candidateMode":"full-catalog"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload debugIssueAnalyzeResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.CandidateSet.Mode != tags.CandidateModeFullCatalog {
		t.Fatalf("expected full-catalog mode, got %q", payload.CandidateSet.Mode)
	}
	for _, tag := range payload.CandidateSet.Tags {
		if !containsCandidateSource(tag.Sources, tags.CandidateSourceFullCatalog) {
			t.Fatalf("expected full-catalog provenance, got %#v", tag)
		}
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

	var payload diagnostics.DebugEvalTagsResult
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Fixture != "corpus" {
		t.Fatalf("expected corpus fixture, got %q", payload.Fixture)
	}
	if payload.CaseCount == 0 {
		t.Fatal("expected at least one benchmark case")
	}
	if payload.CandidateMode != tags.CandidateModeRetrievalShortlist {
		t.Fatalf("expected retrieval-shortlist benchmark mode, got %q", payload.CandidateMode)
	}
	if !payload.VerifierEnabled {
		t.Fatal("expected benchmark verifier to default on")
	}
}

func TestDebugEvalTagsEndpointSupportsFullCatalogMode(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodGet, "/api/ui/debug/eval-tags?mode=full-catalog", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload diagnostics.DebugEvalTagsResult
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.CandidateMode != tags.CandidateModeFullCatalog {
		t.Fatalf("expected full-catalog benchmark mode, got %q", payload.CandidateMode)
	}
}

func TestDebugEvalTagsEndpointSupportsVerifierToggle(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodGet, "/api/ui/debug/eval-tags?verify=off", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload diagnostics.DebugEvalTagsResult
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.VerifierEnabled {
		t.Fatal("expected verifier toggle to disable benchmark verification")
	}
}

func TestDebugFactorWeightsEndpointReturnsReviewQueue(t *testing.T) {
	const reviewIssueID = "issue-review"

	store := newPostgresIssueStore(t, nil)
	if err := store.Replace(context.Background(), []issues.Issue{
		{ID: "issue-a", Raw: "a", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: []float64{1, 0}, CreatedBy: "Casey", CreatedAt: issues.FixtureIssues()[0].CreatedAt},
		{ID: "issue-b", Raw: "b", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: []float64{1, 0}, CreatedBy: "Casey", CreatedAt: issues.FixtureIssues()[1].CreatedAt},
		{ID: "issue-c", Raw: "c", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: []float64{1, 0}, CreatedBy: "Casey", CreatedAt: issues.FixtureIssues()[2].CreatedAt},
		{ID: "issue-d", Raw: "d", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: []float64{1, 0}, CreatedBy: "Casey", CreatedAt: issues.FixtureIssues()[3].CreatedAt},
		{
			ID:        reviewIssueID,
			Raw:       "beta concept hidden behind alpha tag",
			Status:    issues.StatusOpen,
			CreatedBy: "Casey",
			CreatedAt: issues.FixtureIssues()[4].CreatedAt,
			TagScores: []issues.TagRelevance{
				{Tag: "alpha", Relevance: 0.91, Suggested: true, Description: "legacy broad alpha bucket"},
			},
			Embedding: []float64{0, 1},
		},
	}); err != nil {
		t.Fatalf("replace issues: %v", err)
	}
	if err := store.UpsertTags(context.Background(), []issues.Tag{
		{Name: "alpha", Embedding: []float64{1, 0}},
		{Name: "beta", Embedding: []float64{0, 1}},
	}); err != nil {
		t.Fatalf("upsert tags: %v", err)
	}

	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  store,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/ui/debug/factor-weights", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload diagnostics.DebugFactorWeightsResult
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	if len(payload.ReviewQueue.LowR2Issues) == 0 {
		t.Fatal("expected low-R2 review queue entries")
	}
	if payload.ReviewQueue.LowR2Issues[0].ID != reviewIssueID {
		t.Fatalf("expected %s low-R2 entry, got %#v", reviewIssueID, payload.ReviewQueue.LowR2Issues)
	}
	if len(payload.ReviewQueue.LowR2Issues[0].TagScores) != 1 || !payload.ReviewQueue.LowR2Issues[0].TagScores[0].Suggested {
		t.Fatalf("expected suggested tag score provenance, got %#v", payload.ReviewQueue.LowR2Issues[0].TagScores)
	}
	if len(payload.ReviewQueue.LowAlignmentTags) == 0 || payload.ReviewQueue.LowAlignmentTags[0].IssueID != reviewIssueID {
		t.Fatalf("expected low-alignment entry for %s, got %#v", reviewIssueID, payload.ReviewQueue.LowAlignmentTags)
	}
	if len(payload.ReviewQueue.ResidualMisses) == 0 || payload.ReviewQueue.ResidualMisses[0].IssueID != reviewIssueID {
		t.Fatalf("expected residual miss entry for %s, got %#v", reviewIssueID, payload.ReviewQueue.ResidualMisses)
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

func containsCandidateSource(sources []tags.CandidateSource, want tags.CandidateSource) bool {
	for _, source := range sources {
		if source == want {
			return true
		}
	}
	return false
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

	t.Run("analyzer not configured falls back to stub", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPost,
			"/api/ui/debug/issues/analyze",
			bytes.NewBufferString(`{"text":"example"}`),
		)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		// FallbackAnalyzer provides a stub when no real analyzer is configured,
		// so the endpoint succeeds with stub results rather than returning 503.
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})
}
