package queries

import (
	"context"

	"splat/internal/issues"
	issuemap "splat/internal/map"
	"splat/internal/services"
)

type MapQuery struct {
	Viewport      *issuemap.Viewport
	EdgeThreshold *float64
	StatusFilter  IssueStatusFilter
}

type MapHandler struct {
	IssueStore issues.Store
	Catalog    *services.CatalogService
}

func (h MapHandler) Handle(ctx context.Context, input MapQuery) (issuemap.MapResponse, error) {
	storeIssues, err := h.IssueStore.List(ctx)
	if err != nil {
		return issuemap.MapResponse{}, err
	}
	storeIssues = FilterIssuesByStatus(storeIssues, input.StatusFilter)

	storeTags, err := h.Catalog.StoredTags(ctx)
	if err != nil {
		return issuemap.MapResponse{}, err
	}

	if input.EdgeThreshold != nil {
		return issuemap.BuildMapFromIssuesWithTagsAndThreshold(storeIssues, storeTags, input.Viewport, *input.EdgeThreshold)
	}
	return issuemap.BuildMapFromIssuesWithTags(storeIssues, storeTags, input.Viewport)
}

type EdgeHandler struct {
	IssueStore issues.Store
	Catalog    *services.CatalogService
}

func (h EdgeHandler) Handle(ctx context.Context, input MapQuery) (issuemap.EdgeResponse, error) {
	storeIssues, err := h.IssueStore.List(ctx)
	if err != nil {
		return issuemap.EdgeResponse{}, err
	}
	storeIssues = FilterIssuesByStatus(storeIssues, input.StatusFilter)

	storeTags, err := h.Catalog.StoredTags(ctx)
	if err != nil {
		return issuemap.EdgeResponse{}, err
	}

	if input.EdgeThreshold != nil {
		return issuemap.BuildEdgeResponseFromIssuesWithTagsAndThreshold(storeIssues, storeTags, input.Viewport, *input.EdgeThreshold)
	}
	return issuemap.BuildEdgeResponseFromIssuesWithTags(storeIssues, storeTags, input.Viewport)
}
