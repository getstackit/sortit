package issues

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"sortit/internal/domain"
	"sortit/internal/vectors"
)

func (s *InMemoryStore) ListMemories(_ context.Context, opts MemoryListOptions) ([]domain.Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := strings.TrimSpace(string(opts.Status))
	kind := strings.TrimSpace(string(opts.Kind))
	out := make([]domain.Memory, 0, len(s.memories))
	for _, memory := range s.memories {
		if status != "" && string(memory.Status) != status {
			continue
		}
		if kind != "" && string(memory.Kind) != kind {
			continue
		}
		out = append(out, domain.CloneMemory(memory))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})

	if opts.Offset > 0 {
		if opts.Offset >= len(out) {
			return []domain.Memory{}, nil
		}
		out = out[opts.Offset:]
	}
	if opts.Limit > 0 && opts.Limit < len(out) {
		out = out[:opts.Limit]
	}
	return out, nil
}

func (s *InMemoryStore) GetMemory(_ context.Context, id string) (domain.Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	memory, ok := s.memories[strings.TrimSpace(id)]
	if !ok {
		return domain.Memory{}, ErrMemoryNotFound
	}
	return domain.CloneMemory(memory), nil
}

func (s *InMemoryStore) UpsertMemory(_ context.Context, memory domain.Memory) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	memory.ID = strings.TrimSpace(memory.ID)
	now := time.Now().UTC()
	if memory.CreatedAt.IsZero() {
		memory.CreatedAt = now
	}
	if memory.UpdatedAt.IsZero() {
		memory.UpdatedAt = now
	}
	if existing, ok := s.memories[memory.ID]; ok {
		memory.CreatedAt = existing.CreatedAt
		memory.CreatedBy = existing.CreatedBy
	}
	memory.Kind = domain.NormalizeMemoryKind(memory.Kind)
	memory.Status = domain.NormalizeMemoryStatus(memory.Status)
	memory.Source = domain.NormalizeMemorySource(memory.Source)
	memory.SubjectTag = domain.NormalizeSubjectTag(memory.Kind, memory.SubjectTag)
	// Mirror memories_concept_subject_tag_unique_idx: at most one active concept
	// per subject tag.
	if memory.Kind == domain.MemoryKindConcept && memory.Status == domain.MemoryStatusActive {
		for id, other := range s.memories {
			if id != memory.ID && other.Kind == domain.MemoryKindConcept &&
				other.Status == domain.MemoryStatusActive && other.SubjectTag == memory.SubjectTag {
				return fmt.Errorf("active concept already exists for subject tag %q", memory.SubjectTag)
			}
		}
	}
	// Mirror memories_overview_unique_idx: the overview is a global singleton, so
	// at most one active overview may exist.
	if memory.Kind == domain.MemoryKindOverview && memory.Status == domain.MemoryStatusActive {
		for id, other := range s.memories {
			if id != memory.ID && other.Kind == domain.MemoryKindOverview &&
				other.Status == domain.MemoryStatusActive {
				return fmt.Errorf("an active overview already exists")
			}
		}
	}
	s.memories[memory.ID] = domain.CloneMemory(memory)
	return nil
}

func (s *InMemoryStore) GetActiveConceptBySubjectTag(_ context.Context, subjectTag string) (domain.Memory, error) {
	subjectTag = domain.NormalizeTagName(subjectTag)
	if subjectTag == "" {
		return domain.Memory{}, ErrMemoryNotFound
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, memory := range s.memories {
		if memory.Kind == domain.MemoryKindConcept &&
			memory.Status == domain.MemoryStatusActive && memory.SubjectTag == subjectTag {
			return domain.CloneMemory(memory), nil
		}
	}
	return domain.Memory{}, ErrMemoryNotFound
}

func (s *InMemoryStore) GetActiveOverview(_ context.Context) (domain.Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, memory := range s.memories {
		if memory.Kind == domain.MemoryKindOverview && memory.Status == domain.MemoryStatusActive {
			return domain.CloneMemory(memory), nil
		}
	}
	return domain.Memory{}, ErrMemoryNotFound
}

func (s *InMemoryStore) DeleteMemory(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.memories, strings.TrimSpace(id))
	return nil
}

func (s *InMemoryStore) SearchMemories(_ context.Context, query []float64, limit int) ([]MemorySimilarity, error) {
	if len(query) == 0 {
		return []MemorySimilarity{}, nil
	}
	if limit <= 0 {
		limit = 8
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	scored := make([]MemorySimilarity, 0, len(s.memories))
	for _, memory := range s.memories {
		if memory.Status != domain.MemoryStatusActive || len(memory.Embedding) != len(query) {
			continue
		}
		scored = append(scored, MemorySimilarity{
			Memory:     domain.CloneMemory(memory),
			Similarity: vectors.CosineSimilarity(query, memory.Embedding),
		})
	}
	slices.SortStableFunc(scored, func(a, b MemorySimilarity) int {
		if a.Similarity != b.Similarity {
			if a.Similarity > b.Similarity {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Memory.ID, b.Memory.ID)
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return scored, nil
}
