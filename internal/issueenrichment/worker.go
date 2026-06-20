package enrichment

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"sortit/internal/issues"
	"sortit/internal/tags"
)

const (
	defaultIssueEnrichmentPollInterval = 250 * time.Millisecond
	defaultIssueEnrichmentLease        = 30 * time.Second
	defaultIssueEnrichmentRetryDelay   = 10 * time.Second
)

// ConceptReinforcer strengthens the concepts bound to a set of tags. The memory
// service satisfies it; the interface lives here so the enrichment package
// doesn't import memories (memories already imports enrichment).
type ConceptReinforcer interface {
	ReinforceConceptsForTags(ctx context.Context, tags []string) (int, error)
}

type IssueEnrichmentWorker struct {
	Logger        *slog.Logger
	Store         issues.Store
	DB            issues.UnitOfWorkBeginner
	Jobs          issues.EnrichmentJobClaimer
	Enricher      *IssueEnricher
	Catalog       *tags.CatalogService
	Reinforcer    ConceptReinforcer
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

	w.Logger.InfoContext(ctx, "processing enrichment job",
		"issue_id", job.IssueID,
		"target_sequence", job.TargetSequence,
	)

	issue, err := w.Store.Get(ctx, job.IssueID)
	if err != nil {
		w.Logger.WarnContext(ctx, "issue not found for enrichment, completing job",
			"issue_id", job.IssueID,
		)
		if err := w.completeJob(ctx, job); err != nil {
			return true, err
		}
		return true, nil
	}
	if issue.EnrichmentTargetSequence != job.TargetSequence {
		w.Logger.InfoContext(ctx, "enrichment job superseded",
			"issue_id", job.IssueID,
			"job_sequence", job.TargetSequence,
			"current_sequence", issue.EnrichmentTargetSequence,
		)
		if err := w.completeJob(ctx, job); err != nil {
			return true, err
		}
		return true, nil
	}

	fields, err := w.Enricher.AnalyzePersistedIssue(ctx, issue, job.TargetSequence)
	if err != nil {
		w.Logger.ErrorContext(ctx, "enrichment failed, scheduling retry",
			"issue_id", job.IssueID,
			"error", err,
		)
		if retryErr := w.failJob(ctx, job, err); retryErr != nil {
			return true, retryErr
		}
		return true, nil
	}
	if err := w.applyJob(ctx, job, fields); err != nil {
		return true, err
	}
	w.Logger.InfoContext(ctx, "enrichment job completed",
		"issue_id", job.IssueID,
	)
	w.scoreAffectedTags(ctx, fields)
	w.reinforceConcepts(ctx, fields)
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

func (w *IssueEnrichmentWorker) scoreAffectedTags(ctx context.Context, fields issues.IssueFieldUpdate) {
	if w.Catalog == nil || len(fields.TagScores) == 0 {
		return
	}
	for _, ts := range fields.TagScores {
		if err := w.Catalog.ScoreTagSpecificity(ctx, ts.Tag); err != nil {
			w.Logger.WarnContext(ctx, "failed to score tag specificity after enrichment",
				"tag", ts.Tag,
				"error", err,
			)
		}
	}
}

// reinforceConcepts strengthens any concept whose subject tag the just-enriched
// issue carries — the corpus reinforcing its load-bearing nouns. Best-effort and
// non-transactional, mirroring scoreAffectedTags: a failure is logged, never
// fatal to the enrichment job.
func (w *IssueEnrichmentWorker) reinforceConcepts(ctx context.Context, fields issues.IssueFieldUpdate) {
	if w.Reinforcer == nil || len(fields.TagScores) == 0 {
		return
	}
	tagNames := make([]string, 0, len(fields.TagScores))
	for _, ts := range fields.TagScores {
		tagNames = append(tagNames, ts.Tag)
	}
	if _, err := w.Reinforcer.ReinforceConceptsForTags(ctx, tagNames); err != nil {
		w.Logger.WarnContext(ctx, "failed to reinforce concepts after enrichment", "error", err)
	}
}
