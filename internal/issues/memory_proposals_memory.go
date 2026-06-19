package issues

import (
	"context"
	"sort"
	"strings"
	"time"

	"sortit/internal/domain"
)

func (s *InMemoryStore) ListMemoryProposals(_ context.Context, status domain.MemoryProposalStatus) ([]domain.MemoryProposal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	statusFilter := strings.TrimSpace(string(status))
	out := make([]domain.MemoryProposal, 0, len(s.memoryProposals))
	for _, proposal := range s.memoryProposals {
		if statusFilter != "" && string(proposal.Status) != statusFilter {
			continue
		}
		out = append(out, domain.CloneMemoryProposal(proposal))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *InMemoryStore) GetMemoryProposal(_ context.Context, id string) (domain.MemoryProposal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	proposal, ok := s.memoryProposals[strings.TrimSpace(id)]
	if !ok {
		return domain.MemoryProposal{}, ErrMemoryProposalNotFound
	}
	return domain.CloneMemoryProposal(proposal), nil
}

func (s *InMemoryStore) UpsertMemoryProposal(_ context.Context, proposal domain.MemoryProposal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	proposal.ID = strings.TrimSpace(proposal.ID)
	now := time.Now().UTC()
	if proposal.CreatedAt.IsZero() {
		proposal.CreatedAt = now
	}
	if proposal.UpdatedAt.IsZero() {
		proposal.UpdatedAt = now
	}
	if existing, ok := s.memoryProposals[proposal.ID]; ok {
		proposal.CreatedAt = existing.CreatedAt
	}
	proposal.Kind = domain.NormalizeMemoryKind(proposal.Kind)
	proposal.Status = domain.NormalizeMemoryProposalStatus(proposal.Status)
	s.memoryProposals[proposal.ID] = domain.CloneMemoryProposal(proposal)
	return nil
}
