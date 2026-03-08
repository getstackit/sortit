package commands

import (
	"context"

	"splat/internal/issues"
	"splat/internal/services"
)

type replaceableIssueStore interface {
	Replace(context.Context, []issues.Issue) error
}

type LoadSampleIssuesHandler struct {
	Store    replaceableIssueStore
	Enricher *services.IssueEnricher
}

func (h LoadSampleIssuesHandler) Handle(ctx context.Context) (int, error) {
	if h.Store == nil {
		return 0, ErrNotSupported
	}

	samples := issues.SeedIssues()
	enriched, err := h.Enricher.AnalyzeSeedIssues(ctx, samples)
	if err != nil {
		return 0, err
	}
	if err := h.Store.Replace(ctx, enriched); err != nil {
		return 0, err
	}
	return len(samples), nil
}

type ResetIssuesHandler struct {
	Store replaceableIssueStore
}

func (h ResetIssuesHandler) Handle(ctx context.Context) error {
	if h.Store == nil {
		return ErrNotSupported
	}
	return h.Store.Replace(ctx, nil)
}
