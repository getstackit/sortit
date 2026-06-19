// Package memories provides permanent, tag/region-anchored knowledge artifacts.
// Memories reuse the issue math primitives (embedding + tag-relevance profile)
// via the enrichment pipeline, but live in their own store with a permanent,
// reinforced lifecycle so they never pollute the issue corpus they derive from.
package memories

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"sortit/internal/domain"
	issueenrichment "sortit/internal/issueenrichment"
	"sortit/internal/issues"
	"sortit/internal/tags"
)

// anchorTagRelevanceFloor is the minimum relevance for a scored tag to be
// auto-promoted into a memory's anchor set when the author supplied none.
const anchorTagRelevanceFloor = 0.4

// maxDerivedAnchorTags caps how many tags are auto-derived as anchors.
const maxDerivedAnchorTags = 3

// TextEnricher turns raw text into a tag-relevance profile and embedding using
// the existing issue enrichment pipeline. *issueenrichment.IssueEnricher
// satisfies this; it may be nil when no analyzer is configured.
type TextEnricher interface {
	AnalyzeText(ctx context.Context, raw string, opts issueenrichment.AnalyzeTextOptions) (issueenrichment.AnalyzeTextResult, error)
}

// Service is the application layer for memories.
type Service struct {
	store    issues.MemoryStore
	enricher TextEnricher
	logger   *slog.Logger
}

// NewService constructs a memory service. enricher may be nil (memories are
// then persisted without tag scores/embeddings until re-enriched).
func NewService(store issues.MemoryStore, enricher TextEnricher, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, enricher: enricher, logger: logger.With("component", "memories")}
}

// CreateMemoryInput describes a new memory.
type CreateMemoryInput struct {
	Title          string
	Body           string
	Kind           domain.MemoryKind
	AnchorTags     []string
	AnchorRegion   string
	CreatedBy      string
	Source         domain.MemorySource
	SourceIssueIDs []string
	Confidence     float64
}

// CreateMemory validates, enriches, and persists a new memory.
func (s *Service) CreateMemory(ctx context.Context, input CreateMemoryInput) (domain.Memory, error) {
	body, err := issues.ValidateRaw(input.Body, "body")
	if err != nil {
		return domain.Memory{}, err
	}

	confidence := input.Confidence
	if confidence <= 0 {
		confidence = 1
	}

	memory := domain.Memory{
		ID:             issues.NewMemoryID(),
		Title:          strings.TrimSpace(input.Title),
		Body:           body,
		Kind:           domain.NormalizeMemoryKind(input.Kind),
		AnchorTags:     normalizeTagList(input.AnchorTags),
		AnchorRegion:   strings.TrimSpace(input.AnchorRegion),
		Status:         domain.MemoryStatusActive,
		Source:         domain.NormalizeMemorySource(input.Source),
		SourceIssueIDs: normalizeIDList(input.SourceIssueIDs),
		Confidence:     confidence,
		CreatedBy:      strings.TrimSpace(input.CreatedBy),
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	s.enrich(ctx, &memory)

	if err := s.store.UpsertMemory(ctx, memory); err != nil {
		return domain.Memory{}, fmt.Errorf("save memory: %w", err)
	}
	s.logger.InfoContext(ctx, "memory created",
		"memory_id", memory.ID,
		"kind", memory.Kind,
		"anchor_tags", len(memory.AnchorTags),
		"scored_tags", len(memory.TagScores),
		"has_embedding", len(memory.Embedding) > 0,
	)
	return memory, nil
}

// GetMemory returns one memory or issues.ErrMemoryNotFound.
func (s *Service) GetMemory(ctx context.Context, id string) (domain.Memory, error) {
	return s.store.GetMemory(ctx, id)
}

// ListMemories returns memories matching opts, most-recent first.
func (s *Service) ListMemories(ctx context.Context, opts issues.MemoryListOptions) ([]domain.Memory, error) {
	return s.store.ListMemories(ctx, opts)
}

// SupersedeMemory marks a memory as superseded — the "system evolves" path.
// supersededBy optionally records the memory that replaces it.
func (s *Service) SupersedeMemory(ctx context.Context, id, supersededBy string) (domain.Memory, error) {
	return s.transition(ctx, id, domain.MemoryStatusSuperseded, strings.TrimSpace(supersededBy))
}

// ArchiveMemory retires a memory without pointing at a replacement.
func (s *Service) ArchiveMemory(ctx context.Context, id string) (domain.Memory, error) {
	return s.transition(ctx, id, domain.MemoryStatusArchived, "")
}

func (s *Service) transition(ctx context.Context, id string, status domain.MemoryStatus, supersededBy string) (domain.Memory, error) {
	memory, err := s.store.GetMemory(ctx, id)
	if err != nil {
		return domain.Memory{}, err
	}
	if supersededBy != "" && supersededBy == memory.ID {
		return domain.Memory{}, fmt.Errorf("a memory cannot supersede itself")
	}
	memory.Status = status
	memory.SupersededBy = supersededBy
	memory.UpdatedAt = time.Now().UTC()
	if err := s.store.UpsertMemory(ctx, memory); err != nil {
		return domain.Memory{}, fmt.Errorf("save memory transition: %w", err)
	}
	s.logger.InfoContext(ctx, "memory transitioned",
		"memory_id", memory.ID,
		"status", status,
		"superseded_by", supersededBy,
	)
	return memory, nil
}

// enrich runs the memory body through the issue enrichment pipeline to attach a
// tag-relevance profile and embedding. Failures are logged but non-fatal: a
// memory always persists and can be re-enriched later.
func (s *Service) enrich(ctx context.Context, memory *domain.Memory) {
	if s.enricher == nil {
		return
	}
	raw := strings.TrimSpace(strings.TrimSpace(memory.Title) + "\n\n" + memory.Body)
	result, err := s.enricher.AnalyzeText(ctx, raw, issueenrichment.AnalyzeTextOptions{
		PreferredTags: memory.AnchorTags,
		CandidateMode: tags.CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		s.logger.WarnContext(ctx, "memory enrichment failed; persisting without scores", "error", err)
		return
	}
	memory.TagScores = result.TagScores
	memory.Embedding = issueenrichment.Float32VectorToFloat64(result.Analyzed.Embedding.Vector)
	if len(memory.AnchorTags) == 0 {
		memory.AnchorTags = deriveAnchorTags(result.TagScores)
	}
}

// deriveAnchorTags picks the highest-relevance tags as a memory's anchors when
// the author supplied none — the high-value tags the memory centers on.
func deriveAnchorTags(scores []domain.TagRelevance) []string {
	ranked := append([]domain.TagRelevance(nil), scores...)
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].Relevance > ranked[j].Relevance
	})
	out := make([]string, 0, maxDerivedAnchorTags)
	for _, score := range ranked {
		if score.Relevance < anchorTagRelevanceFloor {
			break
		}
		out = append(out, score.Tag)
		if len(out) >= maxDerivedAnchorTags {
			break
		}
	}
	return out
}

func normalizeTagList(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, tag := range in {
		tag = domain.NormalizeTagName(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func normalizeIDList(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, id := range in {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
