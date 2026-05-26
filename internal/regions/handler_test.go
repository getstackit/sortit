package regions

import (
	"context"
	"errors"
	"testing"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

func newTestHandler(items []issues.Issue, now time.Time) *Handler {
	return &Handler{
		Loader: &Loader{Store: &stubStore{items: items}},
		Clock:  func() time.Time { return now },
	}
}

func TestHandlerListDefaultsKindAndWindow(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	h := newTestHandler([]issues.Issue{
		{ID: "a", Status: issues.StatusOpen, CreatedAt: now, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.7}}},
	}, now)

	resp, err := h.List(context.Background(), "", "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if resp.Window.Label != DefaultWindowLabel {
		t.Fatalf("expected default window %q, got %q", DefaultWindowLabel, resp.Window.Label)
	}
	if len(resp.Regions) != 1 || resp.Regions[0].Region.Key.ID != authTag {
		t.Fatalf("expected auth region, got %+v", resp.Regions)
	}
}

func TestHandlerListRejectsUnsupportedKind(t *testing.T) {
	h := newTestHandler(nil, time.Now())
	for _, kind := range []string{"cluster", "custom", "bogus"} {
		t.Run(kind, func(t *testing.T) {
			_, err := h.List(context.Background(), kind, "30d")
			if !errors.Is(err, ErrUnsupportedKind) {
				t.Fatalf("expected ErrUnsupportedKind, got %v", err)
			}
		})
	}
}

func TestHandlerListRejectsUnknownWindow(t *testing.T) {
	h := newTestHandler(nil, time.Now())
	_, err := h.List(context.Background(), "tag", "fortnight")
	if err == nil {
		t.Fatal("expected error for unknown window")
	}
}

func TestHandlerGetReturnsRegion(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	h := newTestHandler([]issues.Issue{
		{ID: "a", Status: issues.StatusOpen, CreatedAt: now, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.7}}},
	}, now)

	resp, err := h.Get(context.Background(), "tag", authTag, "30d")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.Region.Key.ID != authTag {
		t.Fatalf("expected auth, got %q", resp.Region.Key.ID)
	}
	if resp.Metrics.Mass != 1 {
		t.Fatalf("expected mass 1, got %d", resp.Metrics.Mass)
	}
}

func TestHandlerGetReturnsNotFound(t *testing.T) {
	h := newTestHandler(nil, time.Now())
	_, err := h.Get(context.Background(), "tag", "ghost", "30d")
	if !errors.Is(err, ErrRegionNotFound) {
		t.Fatalf("expected ErrRegionNotFound, got %v", err)
	}
}

func TestHandlerGetRejectsUnsupportedKind(t *testing.T) {
	h := newTestHandler(nil, time.Now())
	_, err := h.Get(context.Background(), "cluster", "anything", "30d")
	if !errors.Is(err, ErrUnsupportedKind) {
		t.Fatalf("expected ErrUnsupportedKind, got %v", err)
	}
}
