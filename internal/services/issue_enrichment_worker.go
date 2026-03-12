package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"splat/internal/issues"
)

const (
	defaultIssueEnrichmentPollInterval = 250 * time.Millisecond
	defaultIssueEnrichmentLease        = 30 * time.Second
	defaultIssueEnrichmentRetryDelay   = 10 * time.Second
)

type IssueEnrichmentWorker struct {
	Store         issues.Store
	DB            issues.UnitOfWorkBeginner
	Jobs          issues.EnrichmentJobClaimer
	Enricher      *IssueEnricher
	PollInterval  time.Duration
	LeaseDuration time.Duration
	RetryDelay    time.Duration
	OnStateChange func(context.Context, bool)
}

func (w *IssueEnrichmentWorker) Run(ctx context.Context) error {
	pollInterval := w.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultIssueEnrichmentPollInterval
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		processed, err := w.ProcessOne(ctx)
		if err != nil {
			return err
		}
		if processed {
			continue
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *IssueEnrichmentWorker) ProcessOne(ctx context.Context) (bool, error) {
	if w == nil || w.Store == nil || w.DB == nil || w.Jobs == nil || w.Enricher == nil {
		return false, nil
	}

	leaseDuration := w.LeaseDuration
	if leaseDuration <= 0 {
		leaseDuration = defaultIssueEnrichmentLease
	}
	job, ok, err := w.Jobs.ClaimNextIssueEnrichment(ctx, leaseDuration)
	if err != nil || !ok {
		return ok, err
	}

	issue, err := w.Store.Get(ctx, job.IssueID)
	if err != nil {
		if err := w.completeJob(ctx, job); err != nil {
			return true, err
		}
		return true, nil
	}
	if issue.EnrichmentTargetSequence != job.TargetSequence {
		if err := w.completeJob(ctx, job); err != nil {
			return true, err
		}
		return true, nil
	}

	fields, err := w.Enricher.AnalyzePersistedIssue(ctx, issue, job.TargetSequence)
	if err != nil {
		if retryErr := w.failJob(ctx, job, err); retryErr != nil {
			return true, retryErr
		}
		return true, nil
	}
	if err := w.applyJob(ctx, job, fields); err != nil {
		return true, err
	}
	return true, nil
}

func (w *IssueEnrichmentWorker) applyJob(
	ctx context.Context,
	job issues.IssueEnrichmentJob,
	fields issues.IssueFieldUpdate,
) error {
	changed, err := w.withJobUnitOfWork(ctx, func(uow issues.UnitOfWork, jobWriter issues.EnrichmentJobWriter) (bool, error) {
		current, err := uow.Get(ctx, job.IssueID)
		if err != nil {
			if err := jobWriter.CompleteIssueEnrichment(ctx, job.IssueID, job.TargetSequence); err != nil {
				return false, err
			}
			return false, nil
		}
		if current.EnrichmentTargetSequence != job.TargetSequence {
			if err := jobWriter.CompleteIssueEnrichment(ctx, job.IssueID, job.TargetSequence); err != nil {
				return false, err
			}
			return false, nil
		}
		if err := uow.UpdateIssueFields(ctx, job.IssueID, fields); err != nil {
			return false, err
		}
		if err := jobWriter.CompleteIssueEnrichment(ctx, job.IssueID, job.TargetSequence); err != nil {
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return err
	}
	if changed && w.OnStateChange != nil {
		w.OnStateChange(ctx, true)
	}
	return nil
}

func (w *IssueEnrichmentWorker) failJob(ctx context.Context, job issues.IssueEnrichmentJob, cause error) error {
	retryDelay := w.RetryDelay
	if retryDelay <= 0 {
		retryDelay = defaultIssueEnrichmentRetryDelay
	}
	message := strings.TrimSpace(cause.Error())
	if len(message) > 500 {
		message = message[:500]
	}
	failedStatus := issues.EnrichmentStatusFailed

	changed, err := w.withJobUnitOfWork(ctx, func(uow issues.UnitOfWork, jobWriter issues.EnrichmentJobWriter) (bool, error) {
		current, err := uow.Get(ctx, job.IssueID)
		if err != nil {
			if err := jobWriter.CompleteIssueEnrichment(ctx, job.IssueID, job.TargetSequence); err != nil {
				return false, err
			}
			return false, nil
		}
		if current.EnrichmentTargetSequence != job.TargetSequence {
			if err := jobWriter.CompleteIssueEnrichment(ctx, job.IssueID, job.TargetSequence); err != nil {
				return false, err
			}
			return false, nil
		}
		if err := uow.UpdateIssueFields(ctx, job.IssueID, issues.IssueFieldUpdate{
			EnrichmentStatus: &failedStatus,
			EnrichmentError:  &message,
		}); err != nil {
			return false, err
		}
		if err := jobWriter.RetryIssueEnrichment(ctx, job.IssueID, job.TargetSequence, time.Now().UTC().Add(retryDelay)); err != nil {
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return err
	}
	if changed && w.OnStateChange != nil {
		w.OnStateChange(ctx, false)
	}
	return nil
}

func (w *IssueEnrichmentWorker) completeJob(ctx context.Context, job issues.IssueEnrichmentJob) error {
	_, err := w.withJobUnitOfWork(ctx, func(_ issues.UnitOfWork, jobWriter issues.EnrichmentJobWriter) (bool, error) {
		return false, jobWriter.CompleteIssueEnrichment(ctx, job.IssueID, job.TargetSequence)
	})
	return err
}

func (w *IssueEnrichmentWorker) withJobUnitOfWork(
	ctx context.Context,
	run func(issues.UnitOfWork, issues.EnrichmentJobWriter) (bool, error),
) (changed bool, err error) {
	uow, err := w.DB.BeginUnitOfWork(ctx)
	if err != nil {
		return false, fmt.Errorf("begin enrichment unit of work: %w", err)
	}
	jobWriter, ok := uow.(issues.EnrichmentJobWriter)
	if !ok {
		_ = uow.Rollback()
		return false, fmt.Errorf("unit of work does not support issue enrichment jobs")
	}

	changed, err = run(uow, jobWriter)
	if err != nil {
		_ = uow.Rollback()
		return false, err
	}
	if err := uow.Commit(); err != nil {
		_ = uow.Rollback()
		return false, fmt.Errorf("commit enrichment unit of work: %w", err)
	}
	return changed, nil
}
