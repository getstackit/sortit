package issues

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"sortit/internal/domain"
)

// ErrCustomRegionNotFound is returned when a custom_regions row with the
// requested id doesn't exist.
var ErrCustomRegionNotFound = errors.New("custom region not found")

// CustomRegionStore is the read/write surface for user-defined regions.
type CustomRegionStore interface {
	ListCustomRegions(ctx context.Context) ([]domain.CustomRegion, error)
	GetCustomRegion(ctx context.Context, id string) (domain.CustomRegion, error)
	UpsertCustomRegion(ctx context.Context, region domain.CustomRegion) error
	DeleteCustomRegion(ctx context.Context, id string) error
}

// ListCustomRegions returns every custom region in the database, sorted
// most-recent first.
func (s *PostgresStore) ListCustomRegions(ctx context.Context) ([]domain.CustomRegion, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, label, description, definition_json, created_by,
		        created_at_unix_nano, updated_at_unix_nano
		 FROM custom_regions
		 ORDER BY created_at_unix_nano DESC, id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list custom regions: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	out := make([]domain.CustomRegion, 0)
	for rows.Next() {
		region, err := scanCustomRegion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, region)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate custom regions: %w", err)
	}
	return out, nil
}

// GetCustomRegion returns one region or ErrCustomRegionNotFound.
func (s *PostgresStore) GetCustomRegion(ctx context.Context, id string) (domain.CustomRegion, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.CustomRegion{}, ErrCustomRegionNotFound
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, label, description, definition_json, created_by,
		        created_at_unix_nano, updated_at_unix_nano
		 FROM custom_regions WHERE id = $1`, id,
	)
	region, err := scanCustomRegion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.CustomRegion{}, ErrCustomRegionNotFound
	}
	if err != nil {
		return domain.CustomRegion{}, err
	}
	return region, nil
}

// UpsertCustomRegion inserts or updates a custom region by id.
func (s *PostgresStore) UpsertCustomRegion(ctx context.Context, region domain.CustomRegion) error {
	defn, err := json.Marshal(region.Definition)
	if err != nil {
		return fmt.Errorf("marshal custom region definition: %w", err)
	}
	createdAt := region.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	updatedAt := region.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO custom_regions
		   (id, label, description, definition_json, created_by,
		    created_at_unix_nano, updated_at_unix_nano)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (id) DO UPDATE SET
		   label = EXCLUDED.label,
		   description = EXCLUDED.description,
		   definition_json = EXCLUDED.definition_json,
		   updated_at_unix_nano = EXCLUDED.updated_at_unix_nano`,
		strings.TrimSpace(region.ID),
		strings.TrimSpace(region.Label),
		strings.TrimSpace(region.Description),
		defn,
		strings.TrimSpace(region.CreatedBy),
		createdAt.UTC().UnixNano(),
		updatedAt.UTC().UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("upsert custom region: %w", err)
	}
	return nil
}

// DeleteCustomRegion removes the row by id. No-op if the id doesn't exist.
func (s *PostgresStore) DeleteCustomRegion(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM custom_regions WHERE id = $1`, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("delete custom region: %w", err)
	}
	return nil
}

type customRegionRow interface {
	Scan(dest ...any) error
}

func scanCustomRegion(row customRegionRow) (domain.CustomRegion, error) {
	var (
		id, label, description, createdBy string
		definitionJSON                    []byte
		createdAtNS, updatedAtNS          int64
	)
	if err := row.Scan(&id, &label, &description, &definitionJSON, &createdBy, &createdAtNS, &updatedAtNS); err != nil {
		return domain.CustomRegion{}, err
	}
	var definition domain.CustomRegionPredicate
	if len(definitionJSON) > 0 {
		if err := json.Unmarshal(definitionJSON, &definition); err != nil {
			return domain.CustomRegion{}, fmt.Errorf("decode custom region definition: %w", err)
		}
	}
	return domain.CustomRegion{
		ID:          id,
		Label:       label,
		Description: description,
		Definition:  definition,
		CreatedBy:   createdBy,
		CreatedAt:   time.Unix(0, createdAtNS).UTC(),
		UpdatedAt:   time.Unix(0, updatedAtNS).UTC(),
	}, nil
}
