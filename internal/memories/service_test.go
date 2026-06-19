package memories

import (
	"context"
	"errors"
	"testing"

	"sortit/internal/ai"
	"sortit/internal/domain"
	issueenrichment "sortit/internal/issueenrichment"
	"sortit/internal/issues"
)

type stubEnricher struct {
	result issueenrichment.AnalyzeTextResult
	err    error
	gotRaw string
}

func (s *stubEnricher) AnalyzeText(_ context.Context, raw string, _ issueenrichment.AnalyzeTextOptions) (issueenrichment.AnalyzeTextResult, error) {
	s.gotRaw = raw
	return s.result, s.err
}

func TestCreateMemoryEnrichesAndPersists(t *testing.T) {
	store := issues.NewInMemoryStore(nil)
	enricher := &stubEnricher{
		result: issueenrichment.AnalyzeTextResult{
			Analyzed: ai.AnalyzedIssue{
				Embedding: ai.EmbeddingResult{Vector: []float32{0.1, 0.2, 0.3}},
			},
			TagScores: []domain.TagRelevance{
				{Tag: "search", Relevance: 0.9},
				{Tag: "backend", Relevance: 0.5},
				{Tag: "noise", Relevance: 0.1},
			},
		},
	}
	svc := NewService(store, enricher, nil)

	created, err := svc.CreateMemory(context.Background(), CreateMemoryInput{
		Title: "Ridge default",
		Body:  "Search defaults to the ridge similarity model.",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if created.ID == "" {
		t.Fatal("expected generated id")
	}
	if len(created.Embedding) != 3 {
		t.Fatalf("expected embedding attached, got %v", created.Embedding)
	}
	if len(created.TagScores) != 3 {
		t.Fatalf("expected tag scores attached, got %d", len(created.TagScores))
	}
	if created.Status != domain.MemoryStatusActive {
		t.Fatalf("expected active status, got %s", created.Status)
	}
	if created.Confidence != 1 {
		t.Fatalf("expected default confidence 1, got %v", created.Confidence)
	}
	// Anchor tags auto-derived from high-relevance scores (>= floor), capped.
	if len(created.AnchorTags) != 2 {
		t.Fatalf("expected 2 derived anchor tags, got %v", created.AnchorTags)
	}
	if created.AnchorTags[0] != "search" {
		t.Fatalf("expected highest-relevance tag first, got %v", created.AnchorTags)
	}

	// Enrichment input combines title and body.
	if enricher.gotRaw == "" || enricher.gotRaw == created.Body {
		t.Fatalf("expected enrichment to combine title+body, got %q", enricher.gotRaw)
	}

	persisted, err := store.GetMemory(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get persisted: %v", err)
	}
	if persisted.Title != "Ridge default" {
		t.Fatalf("persisted title mismatch: %q", persisted.Title)
	}
}

func TestCreateMemoryRejectsEmptyBody(t *testing.T) {
	svc := NewService(issues.NewInMemoryStore(nil), nil, nil)
	if _, err := svc.CreateMemory(context.Background(), CreateMemoryInput{Body: "   "}); err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestCreateMemorySurvivesEnrichmentFailure(t *testing.T) {
	store := issues.NewInMemoryStore(nil)
	enricher := &stubEnricher{err: errors.New("analyzer down")}
	svc := NewService(store, enricher, nil)

	created, err := svc.CreateMemory(context.Background(), CreateMemoryInput{
		Body:       "A durable decision.",
		AnchorTags: []string{"search"},
	})
	if err != nil {
		t.Fatalf("create should not fail on enrichment error: %v", err)
	}
	if len(created.Embedding) != 0 || len(created.TagScores) != 0 {
		t.Fatalf("expected no scores when enrichment fails, got %+v", created)
	}
	// Author-supplied anchors are preserved even without enrichment.
	if len(created.AnchorTags) != 1 || created.AnchorTags[0] != "search" {
		t.Fatalf("expected supplied anchor tags preserved, got %v", created.AnchorTags)
	}
}

func TestCreateMemoryNilEnricher(t *testing.T) {
	store := issues.NewInMemoryStore(nil)
	svc := NewService(store, nil, nil)
	created, err := svc.CreateMemory(context.Background(), CreateMemoryInput{Body: "no enricher configured"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(created.Embedding) != 0 {
		t.Fatalf("expected no embedding without enricher, got %v", created.Embedding)
	}
}
