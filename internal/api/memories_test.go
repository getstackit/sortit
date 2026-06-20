package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sortit/internal/ai"
	"sortit/internal/domain"
)

func TestMemorySearchEndpointRecallsMemories(t *testing.T) {
	store := newPostgresIssueStore(t, nil)
	if err := store.UpsertMemory(context.Background(), domain.Memory{
		ID:        "mem-print-pipeline",
		Title:     "Safari export uses the print pipeline",
		Body:      "Use the print pipeline for Safari PDF export.",
		Kind:      domain.MemoryKindDecision,
		Status:    domain.MemoryStatusActive,
		Embedding: []float64{1, 0, 0},
	}); err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		Analyzer: ai.NewAnalyzer(&fakeTagger{}, &fakeEmbedder{
			result: ai.EmbeddingResult{
				Vector: []float32{1, 0, 0},
				Info: ai.EmbeddingInfo{
					Dimensions:          3,
					Preview:             []float32{1, 0, 0},
					ChunkCount:          1,
					EstimatedTokenCount: 3,
				},
			},
		}),
		IssueStore: store,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/ui/memories/search?q=safari%20pdf%20export&limit=5", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var payload recalledMemoriesResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode recall response: %v", err)
	}
	if len(payload.Memories) != 1 {
		t.Fatalf("expected 1 recalled memory, got %#v", payload.Memories)
	}
	if payload.Memories[0].ID != "mem-print-pipeline" {
		t.Fatalf("expected seeded memory, got %q", payload.Memories[0].ID)
	}
	if payload.Memories[0].Similarity <= 0 {
		t.Fatalf("expected positive similarity, got %v", payload.Memories[0].Similarity)
	}
}

func TestMemorySearchEndpointRequiresQuery(t *testing.T) {
	store := newPostgresIssueStore(t, nil)
	server := NewServer(ServerConfig{
		APIPrefixes: []string{"/api"},
		Analyzer: ai.NewAnalyzer(&fakeTagger{}, &fakeEmbedder{
			result: ai.EmbeddingResult{
				Vector: []float32{1, 0, 0},
				Info:   ai.EmbeddingInfo{Dimensions: 3},
			},
		}),
		IssueStore: store,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/ui/memories/search", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing query, got %d (%s)", rec.Code, rec.Body.String())
	}
}
