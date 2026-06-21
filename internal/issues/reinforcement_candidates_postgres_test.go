package issues

import (
	"context"
	"testing"
	"time"
)

// TestPostgresStoreListReinforcementCandidates verifies the slim projection
// behind memory reinforcement scoring: only embedded issues are returned, and
// each carries its most-recent activity time (creation, or closure when later).
func TestPostgresStoreListReinforcementCandidates(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	open := BuildNewIssue(NewIssueID(), CreateInput{Raw: "open with embedding", Embedding: []float64{0.1, 0.9}})
	if err := store.SaveIssue(ctx, open); err != nil {
		t.Fatalf("save open issue: %v", err)
	}

	closed := BuildNewIssue(NewIssueID(), CreateInput{Raw: "closed with embedding", Embedding: []float64{0.3, 0.7}})
	if err := store.SaveIssue(ctx, closed); err != nil {
		t.Fatalf("save closed issue: %v", err)
	}
	closeAt := closed.CreatedAt.Add(time.Hour)
	closedStatus := StatusClosed
	closedBy := "Casey"
	if err := store.UpdateIssueFields(ctx, closed.ID, IssueFieldUpdate{
		Status:   &closedStatus,
		ClosedAt: &closeAt,
		ClosedBy: &closedBy,
	}); err != nil {
		t.Fatalf("close issue: %v", err)
	}

	// An issue without an embedding must be excluded.
	noEmbedding := BuildNewIssue(NewIssueID(), CreateInput{Raw: "no embedding"})
	if err := store.SaveIssue(ctx, noEmbedding); err != nil {
		t.Fatalf("save no-embedding issue: %v", err)
	}

	got, err := store.ListReinforcementCandidates(ctx)
	if err != nil {
		t.Fatalf("list reinforcement candidates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected only the 2 embedded issues, got %d", len(got))
	}

	var sawOpen, sawClosed bool
	for _, c := range got {
		if len(c.Embedding) == 0 {
			t.Fatalf("candidate returned with empty embedding: %+v", c)
		}
		switch c.ActivityAt.UnixNano() {
		case open.CreatedAt.UnixNano():
			sawOpen = true
		case closeAt.UnixNano():
			sawClosed = true
		default:
			t.Fatalf("unexpected activity time %v (open=%v close=%v)", c.ActivityAt, open.CreatedAt, closeAt)
		}
	}
	if !sawOpen {
		t.Fatal("open issue should report its creation time as activity")
	}
	if !sawClosed {
		t.Fatal("closed issue should report its (later) closure time as activity")
	}
}
