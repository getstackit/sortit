package issues

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (s *PostgresStore) EnqueueIssueEnrichment(ctx context.Context, issueID string, targetSequence int) error {
	return enqueueIssueEnrichment(ctx, s.db, issueID, targetSequence)
}

func (u *PostgresUnitOfWork) EnqueueIssueEnrichment(ctx context.Context, issueID string, targetSequence int) error {
	return enqueueIssueEnrichment(ctx, u.tx, issueID, targetSequence)
}

func enqueueIssueEnrichment(ctx context.Context, db execContexter, issueID string, targetSequence int) error {
	issueID = strings.TrimSpace(issueID)
	now := time.Now().UTC().UnixNano()
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO issue_enrichment_jobs (
		     issue_id,
		     target_sequence,
		     lease_expires_at_unix_nano,
		     attempt_count,
		     created_at_unix_nano,
		     updated_at_unix_nano
		 ) VALUES ($1, $2, 0, 0, $3, $3)
		 ON CONFLICT (issue_id) DO UPDATE
		 SET target_sequence = GREATEST(issue_enrichment_jobs.target_sequence, EXCLUDED.target_sequence),
		     lease_expires_at_unix_nano = 0,
		     updated_at_unix_nano = EXCLUDED.updated_at_unix_nano`,
		issueID,
		targetSequence,
		now,
	); err != nil {
		return fmt.Errorf("enqueue issue enrichment for %q: %w", issueID, err)
	}
	return nil
}

func (s *PostgresStore) ClaimNextIssueEnrichment(
	ctx context.Context,
	leaseDuration time.Duration,
) (IssueEnrichmentJob, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IssueEnrichmentJob{}, false, fmt.Errorf("begin enrichment job claim: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now().UTC()
	leaseUntil := now.Add(leaseDuration).UnixNano()
	row := tx.QueryRowContext(
		ctx,
		`WITH next_job AS (
		     SELECT issue_id, target_sequence, attempt_count
		     FROM issue_enrichment_jobs
		     WHERE lease_expires_at_unix_nano <= $1
		     ORDER BY updated_at_unix_nano ASC, issue_id ASC
		     FOR UPDATE SKIP LOCKED
		     LIMIT 1
		 )
		 UPDATE issue_enrichment_jobs AS jobs
		 SET lease_expires_at_unix_nano = $2,
		     attempt_count = next_job.attempt_count + 1,
		     updated_at_unix_nano = $1
		 FROM next_job
		 WHERE jobs.issue_id = next_job.issue_id
		 RETURNING jobs.issue_id, jobs.target_sequence, jobs.attempt_count`,
		now.UnixNano(),
		leaseUntil,
	)

	var job IssueEnrichmentJob
	switch err := row.Scan(&job.IssueID, &job.TargetSequence, &job.AttemptCount); {
	case err == nil:
		if commitErr := tx.Commit(); commitErr != nil {
			return IssueEnrichmentJob{}, false, fmt.Errorf("commit enrichment job claim: %w", commitErr)
		}
		return job, true, nil
	case err == sql.ErrNoRows:
		return IssueEnrichmentJob{}, false, nil
	default:
		return IssueEnrichmentJob{}, false, fmt.Errorf("claim next enrichment job: %w", err)
	}
}

func (s *PostgresStore) CompleteIssueEnrichment(ctx context.Context, issueID string, targetSequence int) error {
	return completeIssueEnrichment(ctx, s.db, issueID, targetSequence)
}

func (u *PostgresUnitOfWork) CompleteIssueEnrichment(ctx context.Context, issueID string, targetSequence int) error {
	return completeIssueEnrichment(ctx, u.tx, issueID, targetSequence)
}

func completeIssueEnrichment(ctx context.Context, db execContexter, issueID string, targetSequence int) error {
	if _, err := db.ExecContext(
		ctx,
		`DELETE FROM issue_enrichment_jobs
		 WHERE issue_id = $1 AND target_sequence = $2`,
		strings.TrimSpace(issueID),
		targetSequence,
	); err != nil {
		return fmt.Errorf("complete issue enrichment for %q: %w", issueID, err)
	}
	return nil
}

func (s *PostgresStore) RetryIssueEnrichment(
	ctx context.Context,
	issueID string,
	targetSequence int,
	nextAttemptAt time.Time,
) error {
	return retryIssueEnrichment(ctx, s.db, issueID, targetSequence, nextAttemptAt)
}

func (u *PostgresUnitOfWork) RetryIssueEnrichment(
	ctx context.Context,
	issueID string,
	targetSequence int,
	nextAttemptAt time.Time,
) error {
	return retryIssueEnrichment(ctx, u.tx, issueID, targetSequence, nextAttemptAt)
}

func retryIssueEnrichment(
	ctx context.Context,
	db execContexter,
	issueID string,
	targetSequence int,
	nextAttemptAt time.Time,
) error {
	if _, err := db.ExecContext(
		ctx,
		`UPDATE issue_enrichment_jobs
		 SET lease_expires_at_unix_nano = $3,
		     updated_at_unix_nano = $3
		 WHERE issue_id = $1 AND target_sequence = $2`,
		strings.TrimSpace(issueID),
		targetSequence,
		nextAttemptAt.UTC().UnixNano(),
	); err != nil {
		return fmt.Errorf("retry issue enrichment for %q: %w", issueID, err)
	}
	return nil
}

type execContexter interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}
