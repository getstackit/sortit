package appendonly

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Checkpoint struct {
	Name        string          `json:"name"`
	Phase       string          `json:"phase"`
	CursorJSON  json.RawMessage `json:"cursorJson"`
	SummaryJSON json.RawMessage `json:"summaryJson"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

type ParityRun struct {
	ID          string          `json:"id"`
	Domain      string          `json:"domain"`
	Status      string          `json:"status"`
	DetailsJSON json.RawMessage `json:"detailsJson"`
	CreatedAt   time.Time       `json:"createdAt"`
}

type FrameworkStore struct {
	db *sql.DB
}

func NewFrameworkStore(db *sql.DB) *FrameworkStore {
	return &FrameworkStore{db: db}
}

func (s *FrameworkStore) UpsertCheckpoint(ctx context.Context, checkpoint Checkpoint) error {
	name := strings.TrimSpace(checkpoint.Name)
	if name == "" {
		return fmt.Errorf("checkpoint name is required")
	}

	phase := strings.TrimSpace(checkpoint.Phase)
	cursorJSON, err := normalizeJSON(checkpoint.CursorJSON)
	if err != nil {
		return fmt.Errorf("normalize checkpoint cursor json: %w", err)
	}
	summaryJSON, err := normalizeJSON(checkpoint.SummaryJSON)
	if err != nil {
		return fmt.Errorf("normalize checkpoint summary json: %w", err)
	}

	updatedAt := checkpoint.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO append_only_migration_checkpoints (
		     name,
		     phase,
		     cursor_json,
		     summary_json,
		     updated_at_unix_nano
		 ) VALUES ($1, $2, $3::jsonb, $4::jsonb, $5)
		 ON CONFLICT (name) DO UPDATE
		 SET phase = EXCLUDED.phase,
		     cursor_json = EXCLUDED.cursor_json,
		     summary_json = EXCLUDED.summary_json,
		     updated_at_unix_nano = EXCLUDED.updated_at_unix_nano`,
		name,
		phase,
		[]byte(cursorJSON),
		[]byte(summaryJSON),
		updatedAt.UnixNano(),
	); err != nil {
		return fmt.Errorf("upsert append-only migration checkpoint %q: %w", name, err)
	}

	return nil
}

func (s *FrameworkStore) GetCheckpoint(ctx context.Context, name string) (Checkpoint, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Checkpoint{}, false, fmt.Errorf("checkpoint name is required")
	}

	var (
		checkpoint Checkpoint
		updatedAt  int64
	)
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT name, phase, cursor_json, summary_json, updated_at_unix_nano
		 FROM append_only_migration_checkpoints
		 WHERE name = $1`,
		name,
	).Scan(
		&checkpoint.Name,
		&checkpoint.Phase,
		&checkpoint.CursorJSON,
		&checkpoint.SummaryJSON,
		&updatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return Checkpoint{}, false, nil
		}
		return Checkpoint{}, false, fmt.Errorf("get append-only migration checkpoint %q: %w", name, err)
	}

	checkpoint.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return checkpoint, true, nil
}

func (s *FrameworkStore) ListCheckpoints(ctx context.Context) ([]Checkpoint, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT name, phase, cursor_json, summary_json, updated_at_unix_nano
		 FROM append_only_migration_checkpoints
		 ORDER BY updated_at_unix_nano DESC, name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list append-only migration checkpoints: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var checkpoints []Checkpoint
	for rows.Next() {
		var (
			checkpoint Checkpoint
			updatedAt  int64
		)
		if err := rows.Scan(
			&checkpoint.Name,
			&checkpoint.Phase,
			&checkpoint.CursorJSON,
			&checkpoint.SummaryJSON,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan append-only migration checkpoint: %w", err)
		}
		checkpoint.UpdatedAt = time.Unix(0, updatedAt).UTC()
		checkpoints = append(checkpoints, checkpoint)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate append-only migration checkpoints: %w", err)
	}
	return checkpoints, nil
}

func (s *FrameworkStore) RecordParityRun(ctx context.Context, run ParityRun) error {
	id := strings.TrimSpace(run.ID)
	if id == "" {
		return fmt.Errorf("parity run id is required")
	}
	domain := strings.TrimSpace(run.Domain)
	if domain == "" {
		return fmt.Errorf("parity run domain is required")
	}
	status := strings.TrimSpace(run.Status)
	if status == "" {
		return fmt.Errorf("parity run status is required")
	}

	detailsJSON, err := normalizeJSON(run.DetailsJSON)
	if err != nil {
		return fmt.Errorf("normalize parity run details json: %w", err)
	}

	createdAt := run.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO append_only_parity_runs (
		     id,
		     domain,
		     status,
		     details_json,
		     created_at_unix_nano
		 ) VALUES ($1, $2, $3, $4::jsonb, $5)`,
		id,
		domain,
		status,
		[]byte(detailsJSON),
		createdAt.UnixNano(),
	); err != nil {
		return fmt.Errorf("record append-only parity run %q: %w", id, err)
	}

	return nil
}

func (s *FrameworkStore) ListParityRuns(ctx context.Context, domain string, limit int) ([]ParityRun, error) {
	domain = strings.TrimSpace(domain)
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, domain, status, details_json, created_at_unix_nano
		 FROM append_only_parity_runs
		 WHERE ($1 = '' OR domain = $1)
		 ORDER BY created_at_unix_nano DESC, id DESC
		 LIMIT $2`,
		domain,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list append-only parity runs: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var runs []ParityRun
	for rows.Next() {
		var (
			run       ParityRun
			createdAt int64
		)
		if err := rows.Scan(
			&run.ID,
			&run.Domain,
			&run.Status,
			&run.DetailsJSON,
			&createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan append-only parity run: %w", err)
		}
		run.CreatedAt = time.Unix(0, createdAt).UTC()
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate append-only parity runs: %w", err)
	}
	return runs, nil
}

func normalizeJSON(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}

	normalized, err := json.Marshal(decoded)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(normalized), nil
}
