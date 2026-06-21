package enrichment

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

func newFrameEnricherWithStore(t *testing.T, store issues.MemoryStore) *IssueEnricher {
	t.Helper()
	enricher := NewIssueEnricher(nil, nil, slog.Default())
	enricher.SetExemplarPool(nil)
	if store != nil {
		enricher.UseMemoryContext(store)
	}
	return enricher
}

func TestProjectConceptFrameNoStoreIsEmpty(t *testing.T) {
	enricher := newFrameEnricherWithStore(t, nil)
	if frame := enricher.projectConceptFrame(context.Background()); !frame.IsEmpty() {
		t.Fatalf("expected empty frame without a memory store, got %+v", frame)
	}
}

func TestProjectConceptFrameNoOverviewOrConceptsIsEmpty(t *testing.T) {
	store := issues.NewInMemoryStore(nil)
	enricher := newFrameEnricherWithStore(t, store)
	if frame := enricher.projectConceptFrame(context.Background()); !frame.IsEmpty() {
		t.Fatalf("expected empty frame with nothing seeded, got %+v", frame)
	}
}

func TestProjectConceptFrameCapAndOrdering(t *testing.T) {
	store := issues.NewInMemoryStore(nil)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Seed more concepts than the cap, with reinforcement counts that should
	// dictate the lead order (most-reinforced first).
	const seeded = conceptFrameMaxConcepts + 5
	for i := 0; i < seeded; i++ {
		if err := store.UpsertMemory(ctx, domain.Memory{
			ID:                 fmt.Sprintf("c%02d", i),
			Body:               fmt.Sprintf("concept %d", i),
			Kind:               domain.MemoryKindConcept,
			SubjectTag:         fmt.Sprintf("tag-%02d", i),
			Status:             domain.MemoryStatusActive,
			ReinforcementCount: i, // higher i => more reinforced
			CreatedAt:          base.Add(time.Duration(i) * time.Hour),
		}); err != nil {
			t.Fatalf("seed concept %d: %v", i, err)
		}
	}

	frame := newFrameEnricherWithStore(t, store).projectConceptFrame(ctx)
	if len(frame.Concepts) != conceptFrameMaxConcepts {
		t.Fatalf("expected frame capped at %d concepts, got %d", conceptFrameMaxConcepts, len(frame.Concepts))
	}
	// Most-reinforced leads, descending — so tag-29 (i=seeded-1) is first.
	if frame.Concepts[0].SubjectTag != fmt.Sprintf("tag-%02d", seeded-1) {
		t.Fatalf("expected the most-reinforced concept first, got %q", frame.Concepts[0].SubjectTag)
	}
	for i := 1; i < len(frame.Concepts); i++ {
		// Subject tags encode reinforcement, so a strictly decreasing tag suffix
		// confirms reinforcement-descending order.
		if frame.Concepts[i-1].SubjectTag <= frame.Concepts[i].SubjectTag {
			t.Fatalf("concepts not ordered by reinforcement desc: %q then %q",
				frame.Concepts[i-1].SubjectTag, frame.Concepts[i].SubjectTag)
		}
	}
}

func TestProjectConceptFrameTruncatesProfile(t *testing.T) {
	store := issues.NewInMemoryStore(nil)
	ctx := context.Background()

	long := strings.Repeat("é", conceptProfileMaxChars+50) // multibyte: rune-safe truncation
	if err := store.UpsertMemory(ctx, domain.Memory{
		ID:         "c-long",
		Body:       long,
		Kind:       domain.MemoryKindConcept,
		SubjectTag: "verbose",
		Status:     domain.MemoryStatusActive,
	}); err != nil {
		t.Fatalf("seed concept: %v", err)
	}

	frame := newFrameEnricherWithStore(t, store).projectConceptFrame(ctx)
	if len(frame.Concepts) != 1 {
		t.Fatalf("expected 1 concept, got %d", len(frame.Concepts))
	}
	profile := frame.Concepts[0].Profile
	if !strings.HasSuffix(profile, "…") {
		t.Fatalf("expected truncated profile to end with an ellipsis, got %q", profile)
	}
	// At most maxRunes content runes + the ellipsis rune.
	if got := len([]rune(profile)); got > conceptProfileMaxChars+1 {
		t.Fatalf("expected at most %d runes, got %d", conceptProfileMaxChars+1, got)
	}
	if !strings.HasPrefix(profile, "é") {
		t.Fatalf("expected multibyte text preserved (not split mid-character), got %q", profile[:8])
	}
}

func TestProjectConceptFrameFallsBackToTitle(t *testing.T) {
	store := issues.NewInMemoryStore(nil)
	ctx := context.Background()

	if err := store.UpsertMemory(ctx, domain.Memory{
		ID:         "c-title-only",
		Title:      "Ridge regression",
		Body:       "",
		Kind:       domain.MemoryKindConcept,
		SubjectTag: "ridge regression",
		Status:     domain.MemoryStatusActive,
	}); err != nil {
		t.Fatalf("seed concept: %v", err)
	}

	frame := newFrameEnricherWithStore(t, store).projectConceptFrame(ctx)
	if len(frame.Concepts) != 1 {
		t.Fatalf("expected 1 concept, got %d", len(frame.Concepts))
	}
	if frame.Concepts[0].Profile != "Ridge regression" {
		t.Fatalf("expected the title as profile when body is empty, got %q", frame.Concepts[0].Profile)
	}
}

func TestProjectConceptFrameOverviewOnly(t *testing.T) {
	store := issues.NewInMemoryStore(nil)
	ctx := context.Background()

	if err := store.UpsertMemory(ctx, domain.Memory{
		ID:     "ov",
		Body:   "Sortit is an issue tracker.",
		Kind:   domain.MemoryKindOverview,
		Status: domain.MemoryStatusActive,
	}); err != nil {
		t.Fatalf("seed overview: %v", err)
	}

	frame := newFrameEnricherWithStore(t, store).projectConceptFrame(ctx)
	if frame.Overview != "Sortit is an issue tracker." {
		t.Fatalf("unexpected overview: %q", frame.Overview)
	}
	if len(frame.Concepts) != 0 {
		t.Fatalf("expected no concepts, got %d", len(frame.Concepts))
	}
	if frame.IsEmpty() {
		t.Fatal("overview-only frame should not be empty")
	}
}
