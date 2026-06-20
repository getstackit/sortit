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

	// embed and embedErr drive EmbedText, used by recall.
	embed    []float64
	embedErr error
	gotEmbed string
}

func (s *stubEnricher) AnalyzeText(_ context.Context, raw string, _ issueenrichment.AnalyzeTextOptions) (issueenrichment.AnalyzeTextResult, error) {
	s.gotRaw = raw
	return s.result, s.err
}

func (s *stubEnricher) EmbedText(_ context.Context, raw string) ([]float64, error) {
	s.gotEmbed = raw
	return s.embed, s.embedErr
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

func TestCreateConceptBindsToSubjectTag(t *testing.T) {
	store := issues.NewInMemoryStore(nil)
	svc := NewService(store, nil, nil)
	ctx := context.Background()

	// A concept requires a subject tag.
	if _, err := svc.CreateMemory(ctx, CreateMemoryInput{
		Body: "a concept without a subject",
		Kind: domain.MemoryKindConcept,
	}); err == nil {
		t.Fatal("expected error creating a concept without a subject tag")
	}

	concept, err := svc.CreateMemory(ctx, CreateMemoryInput{
		Title:      "Ridge regression",
		Body:       "Our search ranking uses diagonal-penalty ridge regression.",
		Kind:       domain.MemoryKindConcept,
		SubjectTag: "Ridge Regression",
	})
	if err != nil {
		t.Fatalf("create concept: %v", err)
	}
	if concept.SubjectTag != "ridge regression" {
		t.Fatalf("expected normalized subject tag, got %q", concept.SubjectTag)
	}
	if len(concept.AnchorTags) == 0 || concept.AnchorTags[0] != "ridge regression" {
		t.Fatalf("expected the subject tag to lead the anchor set, got %v", concept.AnchorTags)
	}

	// A second concept for the same tag supersedes the first (1:1 binding).
	replacement, err := svc.CreateMemory(ctx, CreateMemoryInput{
		Body:       "Updated ridge definition.",
		Kind:       domain.MemoryKindConcept,
		SubjectTag: "ridge regression",
	})
	if err != nil {
		t.Fatalf("create replacement concept: %v", err)
	}
	prior, err := svc.GetMemory(ctx, concept.ID)
	if err != nil {
		t.Fatalf("get prior concept: %v", err)
	}
	if prior.Status != domain.MemoryStatusSuperseded || prior.SupersededBy != replacement.ID {
		t.Fatalf("expected prior concept superseded by replacement, got status=%s supersededBy=%q", prior.Status, prior.SupersededBy)
	}
	if replacement.Status != domain.MemoryStatusActive {
		t.Fatalf("expected replacement active, got %s", replacement.Status)
	}
}

func TestSupersedeMemory(t *testing.T) {
	store := issues.NewInMemoryStore(nil)
	svc := NewService(store, nil, nil)
	ctx := context.Background()

	original, err := svc.CreateMemory(ctx, CreateMemoryInput{Body: "old decision"})
	if err != nil {
		t.Fatalf("create original: %v", err)
	}
	replacement, err := svc.CreateMemory(ctx, CreateMemoryInput{Body: "new decision"})
	if err != nil {
		t.Fatalf("create replacement: %v", err)
	}

	updated, err := svc.SupersedeMemory(ctx, original.ID, replacement.ID)
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}
	if updated.Status != domain.MemoryStatusSuperseded {
		t.Fatalf("expected superseded status, got %s", updated.Status)
	}
	if updated.SupersededBy != replacement.ID {
		t.Fatalf("expected SupersededBy %q, got %q", replacement.ID, updated.SupersededBy)
	}

	if _, err := svc.SupersedeMemory(ctx, original.ID, original.ID); err == nil {
		t.Fatal("expected error superseding a memory by itself")
	}

	if _, err := svc.SupersedeMemory(ctx, "missing", ""); !errors.Is(err, issues.ErrMemoryNotFound) {
		t.Fatalf("expected ErrMemoryNotFound, got %v", err)
	}
}

func TestSynthesizeAcceptAndRejectProposals(t *testing.T) {
	store := issues.NewInMemoryStore([]issues.Issue{
		closedDecision("i1", "search", "default to ridge", "by_design"),
		closedDecision("i2", "search", "still ridge", "by_design"),
		closedDecision("j1", "export", "csv only", "by_design"),
		closedDecision("j2", "export", "no xlsx", "wont_fix"),
	})
	svc := NewService(store, nil, nil)
	svc.UseSynthesis(store, store)
	ctx := context.Background()

	created, err := svc.SynthesizeProposals(ctx)
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("expected 2 proposals (search, export), got %d", len(created))
	}

	// Rerun is idempotent: tags already covered by pending proposals are skipped.
	again, err := svc.SynthesizeProposals(ctx)
	if err != nil {
		t.Fatalf("re-synthesize: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("expected no new proposals on rerun, got %d", len(again))
	}

	// Accept one: it becomes a synthesized memory and the proposal is resolved.
	target := created[0]
	memory, err := svc.AcceptProposal(ctx, target.ID, "alice")
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if memory.Source != domain.MemorySourceSynthesized {
		t.Fatalf("expected synthesized source, got %s", memory.Source)
	}
	if len(memory.SourceIssueIDs) != 2 {
		t.Fatalf("expected provenance carried to memory, got %+v", memory.SourceIssueIDs)
	}

	accepted, err := svc.ListProposals(ctx, domain.MemoryProposalStatusAccepted)
	if err != nil {
		t.Fatalf("list accepted: %v", err)
	}
	if len(accepted) != 1 || accepted[0].AcceptedMemoryID != memory.ID {
		t.Fatalf("expected 1 accepted proposal linked to the memory, got %+v", accepted)
	}

	// Accepting again is a conflict.
	if _, err := svc.AcceptProposal(ctx, target.ID, "alice"); !errors.Is(err, ErrProposalNotPending) {
		t.Fatalf("expected ErrProposalNotPending, got %v", err)
	}

	// Reject the other.
	rejected, err := svc.RejectProposal(ctx, created[1].ID)
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if rejected.Status != domain.MemoryProposalStatusRejected {
		t.Fatalf("expected rejected status, got %s", rejected.Status)
	}
}

func TestSynthesisNotConfigured(t *testing.T) {
	svc := NewService(issues.NewInMemoryStore(nil), nil, nil)
	if _, err := svc.SynthesizeProposals(context.Background()); err == nil {
		t.Fatal("expected error when synthesis is not configured")
	}
}

func TestArchiveMemory(t *testing.T) {
	store := issues.NewInMemoryStore(nil)
	svc := NewService(store, nil, nil)
	ctx := context.Background()

	created, err := svc.CreateMemory(ctx, CreateMemoryInput{Body: "retire me"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	updated, err := svc.ArchiveMemory(ctx, created.ID)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if updated.Status != domain.MemoryStatusArchived {
		t.Fatalf("expected archived status, got %s", updated.Status)
	}
}
