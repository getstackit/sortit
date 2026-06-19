package memoryanalytics

import (
	"math"
	"testing"
	"time"

	"sortit/internal/domain"
	"sortit/internal/scoring"
)

func TestComputeMemoryReinforcement(t *testing.T) {
	now := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	embedding := []float64{1, 0, 0}

	t.Run("no candidates yields zero", func(t *testing.T) {
		got := ComputeMemoryReinforcement(embedding, nil, now)
		if got.Score != 0 || got.EventCount != 0 || got.LastEventAt != nil {
			t.Fatalf("expected empty reinforcement, got %+v", got)
		}
	})

	t.Run("only neighborhood candidates count", func(t *testing.T) {
		candidates := []ReinforcementCandidate{
			{Embedding: []float64{1, 0, 0}, ActivityAt: now.AddDate(0, 0, -1)}, // related
			{Embedding: []float64{0, 1, 0}, ActivityAt: now.AddDate(0, 0, -1)}, // orthogonal, excluded
			{Embedding: []float64{1, 0, 0}, ActivityAt: now.AddDate(0, 0, 5)},  // future, excluded
		}
		got := ComputeMemoryReinforcement(embedding, candidates, now)
		if got.EventCount != 1 {
			t.Fatalf("expected 1 neighborhood event, got %d", got.EventCount)
		}
		if got.Score <= 0 || got.Score > 1 {
			t.Fatalf("score out of range: %v", got.Score)
		}
		if got.LastEventAt == nil {
			t.Fatal("expected LastEventAt to be set")
		}
	})

	t.Run("recent activity reinforces more than old", func(t *testing.T) {
		recent := ComputeMemoryReinforcement(embedding, []ReinforcementCandidate{
			{Embedding: []float64{1, 0, 0}, ActivityAt: now.AddDate(0, 0, -1)},
		}, now)
		old := ComputeMemoryReinforcement(embedding, []ReinforcementCandidate{
			{Embedding: []float64{1, 0, 0}, ActivityAt: now.AddDate(0, 0, -365)},
		}, now)
		if !(recent.Score > old.Score) {
			t.Fatalf("recent (%v) should reinforce more than old (%v)", recent.Score, old.Score)
		}
	})

	t.Run("more activity reinforces more (saturating)", func(t *testing.T) {
		one := ComputeMemoryReinforcement(embedding, []ReinforcementCandidate{
			{Embedding: []float64{1, 0, 0}, ActivityAt: now.AddDate(0, 0, -1)},
		}, now)
		many := ComputeMemoryReinforcement(embedding, []ReinforcementCandidate{
			{Embedding: []float64{1, 0, 0}, ActivityAt: now.AddDate(0, 0, -1)},
			{Embedding: []float64{1, 0, 0}, ActivityAt: now.AddDate(0, 0, -2)},
			{Embedding: []float64{1, 0, 0}, ActivityAt: now.AddDate(0, 0, -3)},
		}, now)
		if !(many.Score > one.Score) {
			t.Fatalf("more activity should reinforce more: one=%v many=%v", one.Score, many.Score)
		}
		if many.Score > 1 {
			t.Fatalf("score must saturate at 1, got %v", many.Score)
		}
	})
}

func TestMemoryProminence(t *testing.T) {
	if got := MemoryProminence(0); got != 1 {
		t.Fatalf("base prominence should be 1.0 (no decay), got %v", got)
	}
	full := MemoryProminence(1)
	if math.Abs(full-(1+scoring.MemoryReinforcementBoost)) > 1e-9 {
		t.Fatalf("full reinforcement prominence should be 1+boost, got %v", full)
	}
	if MemoryProminence(2) != full {
		t.Fatal("prominence should clamp reinforcement to 1")
	}
}

func TestAttachMemoryReinforcement(t *testing.T) {
	stamp := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	memory := domain.Memory{ID: "m1"}
	out := AttachMemoryReinforcement(memory, MemoryReinforcement{Score: 0.7, EventCount: 4, LastEventAt: &stamp})
	if out.ReinforcementCount != 4 {
		t.Fatalf("expected count 4, got %d", out.ReinforcementCount)
	}
	if out.LastReinforcedAt == nil || !out.LastReinforcedAt.Equal(stamp) {
		t.Fatalf("expected last reinforced stamp, got %v", out.LastReinforcedAt)
	}
	// Original is untouched (clone semantics).
	if memory.ReinforcementCount != 0 || memory.LastReinforcedAt != nil {
		t.Fatal("AttachMemoryReinforcement must not mutate the input")
	}
}
