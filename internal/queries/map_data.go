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
	Projection *MapProjectionLoader
}

func (h MapHandler) Handle(ctx context.Context, input MapQuery) (issuemap.MapResponse, error) {
	if h.Projection != nil {
		projection, err := h.Projection.Current(ctx)
		if err != nil {
			return issuemap.MapResponse{}, err
		}
		storeIssues, err := listProjectionIssueMetadata(ctx, h.IssueStore)
		if err != nil {
			return issuemap.MapResponse{}, err
		}
		filteredIssues := FilterIssuesByStatus(storeIssues, input.StatusFilter)
		if len(filteredIssues) < issuemap.MinimumMapIssueCount() {
			return issuemap.UnavailableMapResponse("insufficient_issue_count", len(filteredIssues)), nil
		}
		if projection.UnavailableReason != "" {
			return issuemap.UnavailableMapResponse(projection.UnavailableReason, len(filteredIssues)), nil
		}
		projection = overlayProjectionIssueMetadata(projection, storeIssues)
		projection = subsetProjectionByStatus(projection, input.StatusFilter)
		threshold := issuemapDefaultThreshold(input.EdgeThreshold)
		return issuemap.BuildMapFromProjection(projection, input.Viewport, threshold)
	}
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

func issuemapDefaultThreshold(value *float64) float64 {
	if value == nil {
		return 0.4
	}
	return *value
}
