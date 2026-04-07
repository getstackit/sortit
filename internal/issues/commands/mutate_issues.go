package commands

import (
	"context"
	"fmt"

	"splat/internal/issues"
)

type IssueMutationFailure struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

type IssueMutationResultItem struct {
	ID    string        `json:"id"`
	Issue *issues.Issue `json:"issue,omitempty"`
	Error string        `json:"error,omitempty"`
}

type IssueMutationSummary struct {
	Requested int `json:"requested"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

type IssueMutationResult struct {
	RequestedIDs []string                  `json:"requestedIds"`
	Results      []IssueMutationResultItem `json:"results"`
	Succeeded    []issues.Issue            `json:"succeeded,omitempty"`
	Failed       []IssueMutationFailure    `json:"failed,omitempty"`
	Summary      IssueMutationSummary      `json:"summary"`
}

type AssignIssues struct {
	IDs        []string
	AssignedTo string
}

type AssignIssuesHandler struct {
	AssignIssue AssignIssueHandler
}

func (h AssignIssuesHandler) Handle(ctx context.Context, input AssignIssues) (IssueMutationResult, error) {
	return RunIssueMutationBatch(input.IDs, func(id string) (issues.Issue, error) {
		return h.AssignIssue.Handle(ctx, AssignIssue{
			ID:         id,
			AssignedTo: input.AssignedTo,
		})
	})
}

type CloseIssues struct {
	IDs      []string
	ClosedBy string
}

type CloseIssuesHandler struct {
	CloseIssue CloseIssueHandler
}

func (h CloseIssuesHandler) Handle(ctx context.Context, input CloseIssues) (IssueMutationResult, error) {
	return RunIssueMutationBatch(input.IDs, func(id string) (issues.Issue, error) {
		return h.CloseIssue.Handle(ctx, CloseIssue{
			ID:       id,
			ClosedBy: input.ClosedBy,
		})
	})
}

type ProgressIssues struct {
	IDs       []string
	Raw       string
	CreatedBy string
}

type ProgressIssuesHandler struct {
	ProgressIssue ProgressIssueHandler
}

func (h ProgressIssuesHandler) Handle(ctx context.Context, input ProgressIssues) (IssueMutationResult, error) {
	raw, err := issues.ValidateRaw(input.Raw, "raw")
	if err != nil {
		return IssueMutationResult{}, err
	}

	return RunIssueMutationBatch(input.IDs, func(id string) (issues.Issue, error) {
		return h.ProgressIssue.Handle(ctx, ProgressIssue{
			ID:        id,
			Raw:       raw,
			CreatedBy: input.CreatedBy,
		})
	})
}

type RefineIssues struct {
	IDs       []string
	Raw       string
	CreatedBy string
}

type RefineIssuesHandler struct {
	RefineIssue RefineIssueHandler
}

func (h RefineIssuesHandler) Handle(ctx context.Context, input RefineIssues) (IssueMutationResult, error) {
	raw, err := issues.ValidateRaw(input.Raw, "raw")
	if err != nil {
		return IssueMutationResult{}, err
	}

	return RunIssueMutationBatch(input.IDs, func(id string) (issues.Issue, error) {
		return h.RefineIssue.Handle(ctx, RefineIssue{
			ID:        id,
			Raw:       raw,
			CreatedBy: input.CreatedBy,
		})
	})
}

func RunIssueMutationBatch(ids []string, mutate func(string) (issues.Issue, error)) (IssueMutationResult, error) {
	ids = issues.SanitizeIssueIDs(ids)
	if len(ids) == 0 {
		return IssueMutationResult{}, fmt.Errorf("at least one issue id is required")
	}

	result := IssueMutationResult{
		RequestedIDs: ids,
		Results:      make([]IssueMutationResultItem, 0, len(ids)),
		Succeeded:    make([]issues.Issue, 0, len(ids)),
		Failed:       make([]IssueMutationFailure, 0),
	}

	for _, id := range ids {
		issue, err := mutate(id)
		if err != nil {
			result.Results = append(result.Results, IssueMutationResultItem{
				ID:    id,
				Error: err.Error(),
			})
			result.Failed = append(result.Failed, IssueMutationFailure{
				ID:    id,
				Error: err.Error(),
			})
			continue
		}

		issueCopy := issue
		result.Results = append(result.Results, IssueMutationResultItem{
			ID:    id,
			Issue: &issueCopy,
		})
		result.Succeeded = append(result.Succeeded, issue)
	}

	result.Summary = IssueMutationSummary{
		Requested: len(ids),
		Succeeded: len(result.Succeeded),
		Failed:    len(result.Failed),
	}

	return result, nil
}
