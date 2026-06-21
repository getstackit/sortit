package curation

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/memoryanalytics"
	"sortit/internal/vectors"
)

// MemoryReader is the read surface memory candidate detection rides on.
// issues.MemoryStore satisfies it.
type MemoryReader interface {
	ListMemories(ctx context.Context, opts issues.MemoryListOptions) ([]domain.Memory, error)
}

// ReinforcementSource supplies the slim issue projection quiet-memory detection
// needs: each embedded issue with its most-recent activity time. The production
// store and the in-memory store both implement ListReinforcementCandidates, so
// detection never pulls the full issue hydration just to read embeddings.
type ReinforcementSource interface {
	ListReinforcementCandidates(ctx context.Context) ([]issues.EmbeddingActivity, error)
}

// MemoryDetector surfaces memory-side curation candidates: quiet memories worth
// archiving and redundant memories worth superseding. Like the issue detectors,
// it only reads and computes — it never mutates or proposes.
type MemoryDetector struct {
	issues   ReinforcementSource
	memories MemoryReader
	logger   *slog.Logger
}

// NewMemoryDetector constructs a memory candidate detector.
func NewMemoryDetector(issueReader ReinforcementSource, memories MemoryReader, logger *slog.Logger) *MemoryDetector {
	if logger == nil {
		logger = slog.Default()
	}
	return &MemoryDetector{issues: issueReader, memories: memories, logger: logger.With("component", "curation.detect.memories")}
}

// --- Quiet memories (archive candidates) ---

// QuietMemory is an active memory that recent corpus activity barely reinforces
// — a candidate for archival.
type QuietMemory struct {
	MemoryID           string     `json:"memoryId"`
	Title              string     `json:"title"`
	ReinforcementScore float64    `json:"reinforcementScore"`
	ReinforcementCount int        `json:"reinforcementCount"`
	LastReinforcedAt   *time.Time `json:"lastReinforcedAt,omitempty"`
	AgeDays            float64    `json:"ageDays"`
}

// QuietParams tunes quiet-memory detection. Zero values fall back to defaults.
type QuietParams struct {
	MaxReinforcement float64 // default 0.1; memories at or below are "quiet"
	MinAgeDays       float64 // default 30; younger memories are spared
	Limit            int
}

func (p QuietParams) withDefaults() QuietParams {
	if p.MaxReinforcement <= 0 {
		p.MaxReinforcement = 0.1
	}
	if p.MinAgeDays <= 0 {
		p.MinAgeDays = 30
	}
	return p
}

// --- Redundant memories (supersede candidates) ---

// RedundantMemoryPair is two highly-similar active memories, one of which could
// supersede the other.
type RedundantMemoryPair struct {
	MemoryIDs          []string `json:"memoryIds"`
	Similarity         float64  `json:"similarity"`
	SuggestedSupersede string   `json:"suggestedSupersede"`
	SuggestedKeep      string   `json:"suggestedKeep"`
}

// RedundantParams tunes redundant-memory detection. Zero values fall back to
// defaults.
type RedundantParams struct {
	MinSimilarity float64 // default 0.9
	Limit         int
}

func (p RedundantParams) withDefaults() RedundantParams {
	if p.MinSimilarity <= 0 {
		p.MinSimilarity = 0.9
	}
	return p
}

// MemoryCandidates bundles both memory-side candidate kinds.
type MemoryCandidates struct {
	Quiet     []QuietMemory         `json:"quiet"`
	Redundant []RedundantMemoryPair `json:"redundant"`
}

// DetectQuietMemories scores each active memory's reinforcement against recent
// corpus activity (issue embeddings + creation times) and flags those barely
// reinforced and old enough to retire.
func (d *MemoryDetector) DetectQuietMemories(ctx context.Context, params QuietParams) ([]QuietMemory, error) {
	params = params.withDefaults()
	now := time.Now().UTC()

	mems, err := d.memories.ListMemories(ctx, issues.MemoryListOptions{Status: domain.MemoryStatusActive})
	if err != nil {
		return nil, err
	}
	candidates, err := d.reinforcementCandidates(ctx)
	if err != nil {
		return nil, err
	}

	quiet := make([]QuietMemory, 0)
	for _, mem := range mems {
		if len(mem.Embedding) == 0 {
			continue
		}
		if mem.Kind == domain.MemoryKindConcept {
			// Concepts are durable nodes — the canonical profile of a noun — and
			// stay valuable even when the corpus isn't actively churning around
			// them. Don't propose archiving a concept for being quiet.
			continue
		}
		ageDays := now.Sub(mem.CreatedAt).Hours() / 24
		if ageDays < params.MinAgeDays {
			continue
		}
		r := memoryanalytics.ComputeMemoryReinforcement(mem.Embedding, candidates, now)
		if r.Score > params.MaxReinforcement {
			continue
		}
		quiet = append(quiet, QuietMemory{
			MemoryID:           mem.ID,
			Title:              mem.Title,
			ReinforcementScore: round3(r.Score),
			ReinforcementCount: r.EventCount,
			LastReinforcedAt:   r.LastEventAt,
			AgeDays:            round1(ageDays),
		})
	}

	sort.SliceStable(quiet, func(i, j int) bool { return quiet[i].ReinforcementScore < quiet[j].ReinforcementScore })
	if params.Limit > 0 && len(quiet) > params.Limit {
		quiet = quiet[:params.Limit]
	}
	return quiet, nil
}

func (d *MemoryDetector) reinforcementCandidates(ctx context.Context) ([]memoryanalytics.ReinforcementCandidate, error) {
	rows, err := d.issues.ListReinforcementCandidates(ctx)
	if err != nil {
		return nil, err
	}
	candidates := make([]memoryanalytics.ReinforcementCandidate, 0, len(rows))
	for _, row := range rows {
		if len(row.Embedding) == 0 {
			continue
		}
		candidates = append(candidates, memoryanalytics.ReinforcementCandidate{
			Embedding:  row.Embedding,
			ActivityAt: row.ActivityAt,
		})
	}
	return candidates, nil
}

// DetectRedundantMemories finds pairs of active memories whose embeddings exceed
// the similarity threshold. The higher-confidence (tiebreak newer) memory is the
// suggested keeper; the other is the suggested supersede target.
func (d *MemoryDetector) DetectRedundantMemories(ctx context.Context, params RedundantParams) ([]RedundantMemoryPair, error) {
	params = params.withDefaults()

	mems, err := d.memories.ListMemories(ctx, issues.MemoryListOptions{Status: domain.MemoryStatusActive})
	if err != nil {
		return nil, err
	}

	// We already hold every active memory, so redundancy is a single in-memory
	// O(n^2) similarity sweep over the embedded, non-concept ones — no per-memory
	// vector-search round-trips. Concepts profile a distinct noun (the 1:1
	// subject-tag index already prevents same-tag concept duplicates) and play an
	// orthogonal role to decisions/lessons about the same area, so they are never
	// redundant-supersede candidates.
	candidates := make([]domain.Memory, 0, len(mems))
	for _, mem := range mems {
		if len(mem.Embedding) == 0 || mem.Kind == domain.MemoryKindConcept {
			continue
		}
		candidates = append(candidates, mem)
	}

	pairs := make([]RedundantMemoryPair, 0)
	for i := range candidates {
		a := candidates[i]
		for j := i + 1; j < len(candidates); j++ {
			b := candidates[j]
			if len(a.Embedding) != len(b.Embedding) {
				continue
			}
			sim := vectors.CosineSimilarity(a.Embedding, b.Embedding)
			if sim < params.MinSimilarity {
				continue
			}
			keep, supersede := chooseKeeper(a, b)
			pairs = append(pairs, RedundantMemoryPair{
				MemoryIDs:          []string{keep.ID, supersede.ID},
				Similarity:         round3(sim),
				SuggestedKeep:      keep.ID,
				SuggestedSupersede: supersede.ID,
			})
		}
	}

	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].Similarity > pairs[j].Similarity })
	if params.Limit > 0 && len(pairs) > params.Limit {
		pairs = pairs[:params.Limit]
	}
	return pairs, nil
}

// chooseKeeper returns (keep, supersede): keep the higher-confidence memory,
// breaking ties by the more recently updated one.
func chooseKeeper(a, b domain.Memory) (domain.Memory, domain.Memory) {
	if a.Confidence != b.Confidence {
		if a.Confidence > b.Confidence {
			return a, b
		}
		return b, a
	}
	if a.CreatedAt.After(b.CreatedAt) {
		return a, b
	}
	return b, a
}
