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

// ErrCurationProposalNotFound is returned when a curation_proposals row with the
// requested id doesn't exist.
var ErrCurationProposalNotFound = errors.New("curation proposal not found")

// CurationProposalStore is the read/write surface for the librarian review queue.
type CurationProposalStore interface {
	ListCurationProposals(ctx context.Context, status domain.CurationProposalStatus) ([]domain.CurationProposal, error)
	GetCurationProposal(ctx context.Context, id string) (domain.CurationProposal, error)
	UpsertCurationProposal(ctx context.Context, proposal domain.CurationProposal) error
}

const curationProposalSelectColumns = `id, kind, payload_json, status, rationale, confidence,
	source_refs_json, created_by, reviewed_by, result_ref,
	created_at_unix_nano, updated_at_unix_nano`

// ListCurationProposals returns proposals, optionally filtered by status,
// most-recent first.
func (s *PostgresStore) ListCurationProposals(ctx context.Context, status domain.CurationProposalStatus) ([]domain.CurationProposal, error) {
	statusFilter := strings.TrimSpace(string(status))
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+curationProposalSelectColumns+`
		 FROM curation_proposals
		 WHERE ($1 = '' OR status = $1)
		 ORDER BY created_at_unix_nano DESC, id`,
		statusFilter,
	)
	if err != nil {
		return nil, fmt.Errorf("list curation proposals: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	out := make([]domain.CurationProposal, 0)
	for rows.Next() {
		proposal, err := scanCurationProposal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, proposal)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate curation proposals: %w", err)
	}
	return out, nil
}

// GetCurationProposal returns one proposal or ErrCurationProposalNotFound.
func (s *PostgresStore) GetCurationProposal(ctx context.Context, id string) (domain.CurationProposal, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.CurationProposal{}, ErrCurationProposalNotFound
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+curationProposalSelectColumns+` FROM curation_proposals WHERE id = $1`, id,
	)
	proposal, err := scanCurationProposal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.CurationProposal{}, ErrCurationProposalNotFound
	}
	if err != nil {
		return domain.CurationProposal{}, err
	}
	return proposal, nil
}

// UpsertCurationProposal inserts or updates a proposal by id. CreatedAt is
// preserved on update.
func (s *PostgresStore) UpsertCurationProposal(ctx context.Context, proposal domain.CurationProposal) error {
	payloadJSON, err := marshalJSONB(proposal.Payload, domain.CurationPayload{})
	if err != nil {
		return fmt.Errorf("marshal curation payload: %w", err)
	}
	sourceRefsJSON, err := marshalJSONB(proposal.SourceRefs, []string{})
	if err != nil {
		return fmt.Errorf("marshal curation source refs: %w", err)
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
		`INSERT INTO curation_proposals
		   (id, kind, payload_json, status, rationale, confidence, source_refs_json,
		    created_by, reviewed_by, result_ref,
		    created_at_unix_nano, updated_at_unix_nano)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		 ON CONFLICT (id) DO UPDATE SET
		   kind = EXCLUDED.kind,
		   payload_json = EXCLUDED.payload_json,
		   status = EXCLUDED.status,
		   rationale = EXCLUDED.rationale,
		   confidence = EXCLUDED.confidence,
		   source_refs_json = EXCLUDED.source_refs_json,
		   created_by = EXCLUDED.created_by,
		   reviewed_by = EXCLUDED.reviewed_by,
		   result_ref = EXCLUDED.result_ref,
		   updated_at_unix_nano = EXCLUDED.updated_at_unix_nano`,
		strings.TrimSpace(proposal.ID),
		string(domain.NormalizeCurationKind(proposal.Kind)),
		payloadJSON,
		string(domain.NormalizeCurationProposalStatus(proposal.Status)),
		strings.TrimSpace(proposal.Rationale),
		proposal.Confidence,
		sourceRefsJSON,
		strings.TrimSpace(proposal.CreatedBy),
		strings.TrimSpace(proposal.ReviewedBy),
		strings.TrimSpace(proposal.ResultRef),
		createdAt.UTC().UnixNano(),
		updatedAt.UTC().UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("upsert curation proposal: %w", err)
	}
	return nil
}

func scanCurationProposal(row memoryRow) (domain.CurationProposal, error) {
	var (
		id, kind, status, rationale, createdBy, reviewedBy, resultRef string
		payloadJSON, sourceRefsJSON                                   []byte
		confidence                                                    float64
		createdAtNS, updatedAtNS                                      int64
	)
	if err := row.Scan(
		&id, &kind, &payloadJSON, &status, &rationale, &confidence,
		&sourceRefsJSON, &createdBy, &reviewedBy, &resultRef,
		&createdAtNS, &updatedAtNS,
	); err != nil {
		return domain.CurationProposal{}, err
	}

	payload, err := unmarshalJSONB[domain.CurationPayload](payloadJSON)
	if err != nil {
		return domain.CurationProposal{}, fmt.Errorf("decode curation payload for %q: %w", id, err)
	}
	sourceRefs, err := unmarshalJSONB[[]string](sourceRefsJSON)
	if err != nil {
		return domain.CurationProposal{}, fmt.Errorf("decode curation source refs for %q: %w", id, err)
	}

	return domain.CurationProposal{
		ID:         id,
		Kind:       domain.NormalizeCurationKind(domain.CurationKind(kind)),
		Payload:    payload,
		Status:     domain.NormalizeCurationProposalStatus(domain.CurationProposalStatus(status)),
		Rationale:  rationale,
		Confidence: confidence,
		SourceRefs: sourceRefs,
		CreatedBy:  createdBy,
		ReviewedBy: reviewedBy,
		ResultRef:  resultRef,
		CreatedAt:  time.Unix(0, createdAtNS).UTC(),
		UpdatedAt:  time.Unix(0, updatedAtNS).UTC(),
	}, nil
}
