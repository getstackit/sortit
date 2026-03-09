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

type RefineIssue struct {
	ID        string
	Raw       string
	CreatedBy string
}

type RefineIssueHandler struct {
	Store    issues.Store
	Enricher *services.IssueEnricher
}

func (h RefineIssueHandler) Handle(ctx context.Context, input RefineIssue) (issues.Issue, error) {
	current, err := h.Store.Get(ctx, input.ID)
	if err != nil {
		return issues.Issue{}, err
	}

	enriched, err := h.Enricher.AnalyzeRefineInput(ctx, current, input.Raw, input.CreatedBy)
	if err != nil {
		return issues.Issue{}, err
	}

	return h.Store.Refine(ctx, input.ID, enriched)
}

type ProgressIssue struct {
	ID        string
	Raw       string
	CreatedBy string
}

type ProgressIssueHandler struct {
	Store issues.Store
}

func (h ProgressIssueHandler) Handle(ctx context.Context, input ProgressIssue) (issues.Issue, error) {
	return h.Store.ProgressPost(ctx, input.ID, issues.ProgressInput{
		Raw:       input.Raw,
		CreatedBy: input.CreatedBy,
	})
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

type AssignIssue struct {
	ID         string
	AssignedTo string
}

type AssignIssueHandler struct {
	Store issues.Store
}

func (h AssignIssueHandler) Handle(ctx context.Context, input AssignIssue) (issues.Issue, error) {
	return h.Store.AssignIssue(ctx, input.ID, input.AssignedTo)
}

type SplitIssueChild struct {
	Raw  string
	Tags []string
}

type SplitIssue struct {
	SourceID    string
	Children    []SplitIssueChild
	CreatedBy   string
	Note        string
	CloseSource bool
}

type SplitIssueHandler struct {
	Store    issues.Store
	Enricher *services.IssueEnricher
}

func (h SplitIssueHandler) Handle(ctx context.Context, input SplitIssue) (issues.IssueOperationResult, error) {
	children := make([]issues.SplitChildInput, 0, len(input.Children))
	for _, child := range input.Children {
		enriched, err := h.Enricher.AnalyzeCreateInput(ctx, issues.CreateInput{
			Raw:       child.Raw,
			Tags:      child.Tags,
			CreatedBy: input.CreatedBy,
		})
		if err != nil {
			return issues.IssueOperationResult{}, err
		}
		children = append(children, issues.SplitChildInput{
			Raw:       enriched.Raw,
			Tags:      enriched.Tags,
			TagScores: enriched.TagScores,
			Embedding: enriched.Embedding,
		})
	}

	return h.Store.SplitIssue(ctx, issues.SplitInput{
		SourceID:    input.SourceID,
		Children:    children,
		CreatedBy:   input.CreatedBy,
		Note:        input.Note,
		CloseSource: input.CloseSource,
	})
}

type CombineIssues struct {
	SourceIDs []string
	CreatedBy string
	Note      string
}

type CombineIssuesHandler struct {
	Store    issues.Store
	Enricher *services.IssueEnricher
}

func (h CombineIssuesHandler) Handle(ctx context.Context, input CombineIssues) (issues.IssueOperationResult, error) {
	sourceIssues := make([]issues.Issue, 0, len(input.SourceIDs))
	for _, id := range input.SourceIDs {
		issue, err := h.Store.Get(ctx, id)
		if err != nil {
			return issues.IssueOperationResult{}, err
		}
		sourceIssues = append(sourceIssues, issue)
	}

	enriched, err := h.Enricher.AnalyzeCombineInput(ctx, sourceIssues, input.CreatedBy, input.Note)
	if err != nil {
		return issues.IssueOperationResult{}, err
	}

	return h.Store.CombineIssues(ctx, enriched)
}

type LinkIssues struct {
	SourceID  string
	TargetID  string
	Type      issues.IssueLinkType
	CreatedBy string
	Note      string
}

type LinkIssuesHandler struct {
	Store issues.Store
}

func (h LinkIssuesHandler) Handle(ctx context.Context, input LinkIssues) (issues.IssueOperationResult, error) {
	return h.Store.LinkIssues(ctx, issues.LinkInput{
		SourceID:  input.SourceID,
		TargetID:  input.TargetID,
		Type:      input.Type,
		CreatedBy: input.CreatedBy,
		Note:      input.Note,
	})
}
