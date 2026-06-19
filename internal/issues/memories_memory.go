package issues

import (
	"context"
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
	out := make([]domain.Memory, 0, len(s.memories))
	for _, memory := range s.memories {
		if status != "" && string(memory.Status) != status {
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
	s.memories[memory.ID] = domain.CloneMemory(memory)
	return nil
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
