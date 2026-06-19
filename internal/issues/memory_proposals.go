package issues

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"sortit/internal/domain"
)

// ErrMemoryProposalNotFound is returned when a memory_proposals row with the
// requested id doesn't exist.
var ErrMemoryProposalNotFound = errors.New("memory proposal not found")

// MemoryProposalStore is the read/write surface for the synthesis review queue.
type MemoryProposalStore interface {
	ListMemoryProposals(ctx context.Context, status domain.MemoryProposalStatus) ([]domain.MemoryProposal, error)
	GetMemoryProposal(ctx context.Context, id string) (domain.MemoryProposal, error)
	UpsertMemoryProposal(ctx context.Context, proposal domain.MemoryProposal) error
}

const memoryProposalSelectColumns = `id, title, body, kind, anchor_tags_json, anchor_region,
	source_issue_ids_json, confidence, status, rationale, accepted_memory_id,
	created_at_unix_nano, updated_at_unix_nano`

// ListMemoryProposals returns proposals, optionally filtered by status,
// most-recent first.
func (s *PostgresStore) ListMemoryProposals(ctx context.Context, status domain.MemoryProposalStatus) ([]domain.MemoryProposal, error) {
	statusFilter := strings.TrimSpace(string(status))
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+memoryProposalSelectColumns+`
		 FROM memory_proposals
		 WHERE ($1 = '' OR status = $1)
		 ORDER BY created_at_unix_nano DESC, id`,
		statusFilter,
	)
	if err != nil {
		return nil, fmt.Errorf("list memory proposals: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	out := make([]domain.MemoryProposal, 0)
	for rows.Next() {
		proposal, err := scanMemoryProposal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, proposal)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate memory proposals: %w", err)
	}
	return out, nil
}

// GetMemoryProposal returns one proposal or ErrMemoryProposalNotFound.
func (s *PostgresStore) GetMemoryProposal(ctx context.Context, id string) (domain.MemoryProposal, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.MemoryProposal{}, ErrMemoryProposalNotFound
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+memoryProposalSelectColumns+` FROM memory_proposals WHERE id = $1`, id,
	)
	proposal, err := scanMemoryProposal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.MemoryProposal{}, ErrMemoryProposalNotFound
	}
	if err != nil {
		return domain.MemoryProposal{}, err
	}
	return proposal, nil
}

// UpsertMemoryProposal inserts or updates a proposal by id. CreatedAt is
// preserved on update.
func (s *PostgresStore) UpsertMemoryProposal(ctx context.Context, proposal domain.MemoryProposal) error {
	anchorTagsJSON, err := marshalJSONB(proposal.AnchorTags, []string{})
	if err != nil {
		return fmt.Errorf("marshal proposal anchor tags: %w", err)
	}
	sourceIssueIDsJSON, err := marshalJSONB(proposal.SourceIssueIDs, []string{})
	if err != nil {
		return fmt.Errorf("marshal proposal source issue ids: %w", err)
	}
	createdAt := proposal.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	updatedAt := proposal.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO memory_proposals
		   (id, title, body, kind, anchor_tags_json, anchor_region, source_issue_ids_json,
		    confidence, status, rationale, accepted_memory_id,
		    created_at_unix_nano, updated_at_unix_nano)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		 ON CONFLICT (id) DO UPDATE SET
		   title = EXCLUDED.title,
		   body = EXCLUDED.body,
		   kind = EXCLUDED.kind,
		   anchor_tags_json = EXCLUDED.anchor_tags_json,
		   anchor_region = EXCLUDED.anchor_region,
		   source_issue_ids_json = EXCLUDED.source_issue_ids_json,
		   confidence = EXCLUDED.confidence,
		   status = EXCLUDED.status,
		   rationale = EXCLUDED.rationale,
		   accepted_memory_id = EXCLUDED.accepted_memory_id,
		   updated_at_unix_nano = EXCLUDED.updated_at_unix_nano`,
		strings.TrimSpace(proposal.ID),
		strings.TrimSpace(proposal.Title),
		strings.TrimSpace(proposal.Body),
		string(domain.NormalizeMemoryKind(proposal.Kind)),
		anchorTagsJSON,
		strings.TrimSpace(proposal.AnchorRegion),
		sourceIssueIDsJSON,
		proposal.Confidence,
		string(domain.NormalizeMemoryProposalStatus(proposal.Status)),
		strings.TrimSpace(proposal.Rationale),
		strings.TrimSpace(proposal.AcceptedMemoryID),
		createdAt.UTC().UnixNano(),
		updatedAt.UTC().UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("upsert memory proposal: %w", err)
	}
	return nil
}

func scanMemoryProposal(row memoryRow) (domain.MemoryProposal, error) {
	var (
		id, title, body, kind, anchorRegion, status, rationale, acceptedMemoryID string
		anchorTagsJSON, sourceIssueIDsJSON                                       []byte
		confidence                                                               float64
		createdAtNS, updatedAtNS                                                 int64
	)
	if err := row.Scan(
		&id, &title, &body, &kind, &anchorTagsJSON, &anchorRegion,
		&sourceIssueIDsJSON, &confidence, &status, &rationale, &acceptedMemoryID,
		&createdAtNS, &updatedAtNS,
	); err != nil {
		return domain.MemoryProposal{}, err
	}

	anchorTags, err := unmarshalJSONB[[]string](anchorTagsJSON)
	if err != nil {
		return domain.MemoryProposal{}, fmt.Errorf("decode proposal anchor tags for %q: %w", id, err)
	}
	sourceIssueIDs, err := unmarshalJSONB[[]string](sourceIssueIDsJSON)
	if err != nil {
		return domain.MemoryProposal{}, fmt.Errorf("decode proposal source issue ids for %q: %w", id, err)
	}

	return domain.MemoryProposal{
		ID:               id,
		Title:            title,
		Body:             body,
		Kind:             domain.NormalizeMemoryKind(domain.MemoryKind(kind)),
		AnchorTags:       anchorTags,
		AnchorRegion:     anchorRegion,
		SourceIssueIDs:   sourceIssueIDs,
		Confidence:       confidence,
		Status:           domain.NormalizeMemoryProposalStatus(domain.MemoryProposalStatus(status)),
		Rationale:        rationale,
		AcceptedMemoryID: acceptedMemoryID,
		CreatedAt:        time.Unix(0, createdAtNS).UTC(),
		UpdatedAt:        time.Unix(0, updatedAtNS).UTC(),
	}, nil
}
