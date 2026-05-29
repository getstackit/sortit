package issues

import (
	"context"
	"sort"
	"strings"
	"time"

	"sortit/internal/domain"
)

func (s *InMemoryStore) ListCustomRegions(_ context.Context) ([]domain.CustomRegion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.CustomRegion, 0, len(s.customRegions))
	for _, region := range s.customRegions {
		out = append(out, cloneCustomRegion(region))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *InMemoryStore) GetCustomRegion(_ context.Context, id string) (domain.CustomRegion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id = strings.TrimSpace(id)
	region, ok := s.customRegions[id]
	if !ok {
		return domain.CustomRegion{}, ErrCustomRegionNotFound
	}
	return cloneCustomRegion(region), nil
}

func (s *InMemoryStore) UpsertCustomRegion(_ context.Context, region domain.CustomRegion) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	region.ID = strings.TrimSpace(region.ID)
	if region.CreatedAt.IsZero() {
		region.CreatedAt = time.Now().UTC()
	}
	if region.UpdatedAt.IsZero() {
		region.UpdatedAt = time.Now().UTC()
	}
	if existing, ok := s.customRegions[region.ID]; ok {
		region.CreatedAt = existing.CreatedAt
		region.CreatedBy = existing.CreatedBy
	}
	s.customRegions[region.ID] = cloneCustomRegion(region)
	return nil
}

func (s *InMemoryStore) DeleteCustomRegion(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.customRegions, strings.TrimSpace(id))
	return nil
}

func cloneCustomRegion(region domain.CustomRegion) domain.CustomRegion {
	out := region
	if len(region.Definition.Tags) > 0 {
		out.Definition.Tags = append([]domain.TagPredicate(nil), region.Definition.Tags...)
	}
	if len(region.Definition.Statuses) > 0 {
		out.Definition.Statuses = append([]string(nil), region.Definition.Statuses...)
	}
	if len(region.Definition.Authors) > 0 {
		out.Definition.Authors = append([]string(nil), region.Definition.Authors...)
	}
	if region.Definition.MaxAgeDays != nil {
		v := *region.Definition.MaxAgeDays
		out.Definition.MaxAgeDays = &v
	}
	return out
}
