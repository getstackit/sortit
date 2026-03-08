package commands

import (
	"context"

	"splat/internal/issues"
	"splat/internal/services"
)

type CreateIssue struct {
	Raw       string
	Tags      []string
	CreatedBy string
}

type CreateIssueHandler struct {
	Store    issues.Store
	Enricher *services.IssueEnricher
}

func (h CreateIssueHandler) Handle(ctx context.Context, input CreateIssue) (issues.Issue, error) {
	enriched, err := h.Enricher.AnalyzeCreateInput(ctx, issues.CreateInput{
		Raw:       input.Raw,
		Tags:      input.Tags,
		CreatedBy: input.CreatedBy,
	})
	if err != nil {
		return issues.Issue{}, err
	}
	return h.Store.Create(ctx, enriched)
}

type CloseIssue struct {
	ID       string
	ClosedBy string
}

type CloseIssueHandler struct {
	Store issues.Store
}

func (h CloseIssueHandler) Handle(ctx context.Context, input CloseIssue) (issues.Issue, error) {
	return h.Store.CloseIssue(ctx, input.ID, input.ClosedBy)
}

type ReopenIssue struct {
	ID string
}

type ReopenIssueHandler struct {
	Store issues.Store
}

func (h ReopenIssueHandler) Handle(ctx context.Context, input ReopenIssue) (issues.Issue, error) {
	return h.Store.ReopenIssue(ctx, input.ID)
}
