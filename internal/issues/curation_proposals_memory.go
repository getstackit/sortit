package issues

import (
	"context"
	"sort"
	"strings"
	"time"

	"sortit/internal/domain"
)

func (s *InMemoryStore) ListCurationProposals(_ context.Context, status domain.CurationProposalStatus) ([]domain.CurationProposal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	statusFilter := strings.TrimSpace(string(status))
	out := make([]domain.CurationProposal, 0, len(s.curationProposals))
	for _, proposal := range s.curationProposals {
		if statusFilter != "" && string(proposal.Status) != statusFilter {
			continue
		}
		out = append(out, domain.CloneCurationProposal(proposal))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *InMemoryStore) GetCurationProposal(_ context.Context, id string) (domain.CurationProposal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	proposal, ok := s.curationProposals[strings.TrimSpace(id)]
	if !ok {
		return domain.CurationProposal{}, ErrCurationProposalNotFound
	}
	return domain.CloneCurationProposal(proposal), nil
}

func (s *InMemoryStore) UpsertCurationProposal(_ context.Context, proposal domain.CurationProposal) error {
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
	if existing, ok := s.curationProposals[proposal.ID]; ok {
		proposal.CreatedAt = existing.CreatedAt
	}
	proposal.Kind = domain.NormalizeCurationKind(proposal.Kind)
	proposal.Status = domain.NormalizeCurationProposalStatus(proposal.Status)
	s.curationProposals[proposal.ID] = domain.CloneCurationProposal(proposal)
	return nil
}
