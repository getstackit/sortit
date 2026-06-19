// Package memoryanalytics computes the temporal model for memories. Where
// issues decay (see issueanalytics.IssueFreshnessWeight), memories exhibit
// reinforced permanence: their base prominence never fades, and recent related
// corpus activity strengthens them. This inverts the velocity idea — activity
// reinforces rather than freshens.
package memoryanalytics

import (
	"math"
	"time"

	"sortit/internal/domain"
	"sortit/internal/scoring"
	"sortit/internal/vectors"
)

// ReinforcementCandidate is a recent corpus activity (e.g. an issue created,
// refined, or closed) that reinforces a memory when it falls inside the
// memory's embedding neighborhood.
type ReinforcementCandidate struct {
	Embedding  []float64
	ActivityAt time.Time
}

// MemoryReinforcement is the result of evaluating recent activity against a
// memory. Score is the saturated reinforcement strength in [0,1].
type MemoryReinforcement struct {
	Score       float64
	EventCount  int
	LastEventAt *time.Time
}

// ComputeMemoryReinforcement measures how strongly recent related activity
// reinforces a memory. Each candidate within the similarity threshold
// contributes its relatedness, decayed by a long half-life and saturated, so a
// memory in an active area scores near 1 while one in a quiet area scores near
// 0. There is no window cutoff — the long half-life drives stale activity
// toward zero smoothly, matching ComputeIssueVelocity's design.
func ComputeMemoryReinforcement(memoryEmbedding []float64, candidates []ReinforcementCandidate, now time.Time) MemoryReinforcement {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if len(memoryEmbedding) == 0 {
		return MemoryReinforcement{}
	}

	var (
		weighted float64
		count    int
		last     time.Time
	)
	for _, candidate := range candidates {
		if len(candidate.Embedding) != len(memoryEmbedding) {
			continue
		}
		at := candidate.ActivityAt.UTC()
		if at.IsZero() || at.After(now) {
			continue
		}
		sim := vectors.CosineSimilarity(memoryEmbedding, candidate.Embedding)
		if sim < scoring.MemoryNeighborhoodSimThreshold {
			continue
		}
		ageDays := now.Sub(at).Hours() / 24
		decay := math.Exp(-math.Ln2 * ageDays / scoring.MemoryReinforcementHalfLifeDays)
		weighted += sim * decay
		count++
		if at.After(last) {
			last = at
		}
	}

	if weighted == 0 {
		return MemoryReinforcement{}
	}

	result := MemoryReinforcement{
		Score:      clamp01(1 - math.Exp(-weighted/scoring.MemoryReinforcementSaturation)),
		EventCount: count,
	}
	if !last.IsZero() {
		stamp := last
		result.LastEventAt = &stamp
	}
	return result
}

// MemoryProminence converts a reinforcement score into a multiplicative
// prominence weight. The base is 1.0 — memories never decay — and reinforcement
// adds up to MemoryReinforcementBoost on top.
func MemoryProminence(reinforcement float64) float64 {
	return 1 + scoring.MemoryReinforcementBoost*clamp01(reinforcement)
}

// AttachMemoryReinforcement records a computed reinforcement on a memory for
// read-time display: the neighborhood event count and the most recent related
// activity timestamp. It returns a clone and does not persist.
func AttachMemoryReinforcement(memory domain.Memory, reinforcement MemoryReinforcement) domain.Memory {
	out := domain.CloneMemory(memory)
	out.ReinforcementCount = reinforcement.EventCount
	out.LastReinforcedAt = reinforcement.LastEventAt
	return out
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
