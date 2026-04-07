package mapview

import (
	"context"

	"splat/internal/issues"
	issueviews "splat/internal/issues/views"
	issuemap "splat/internal/map"
	"splat/internal/tags"
)

type MapQuery struct {
	Viewport      *issuemap.Viewport
	EdgeThreshold *float64
	StatusFilter  issueviews.IssueStatusFilter
}

type MapHandler struct {
	IssueStore issues.Store
	Catalog    *tags.CatalogService
	Projection *MapProjectionLoader
}

func (h MapHandler) Handle(ctx context.Context, input MapQuery) (issuemap.MapResponse, error) {
	if h.Projection != nil {
		projection, err := h.Projection.Current(ctx)
		if err != nil {
			return issuemap.MapResponse{}, err
		}
		storeIssues, err := issueviews.ListProjectionIssueMetadata(ctx, h.IssueStore)
		if err != nil {
			return issuemap.MapResponse{}, err
		}
		filteredIssues := issueviews.FilterIssuesByStatus(storeIssues, input.StatusFilter)
		if len(filteredIssues) < issuemap.MinimumMapIssueCount() {
			return issuemap.UnavailableMapResponse("insufficient_issue_count", len(filteredIssues)), nil
		}
		if projection.UnavailableReason != "" {
			return issuemap.UnavailableMapResponse(projection.UnavailableReason, len(filteredIssues)), nil
		}
		projection = issueviews.OverlayProjectionIssueMetadata(projection, storeIssues)
		projection = issueviews.SubsetProjectionByStatus(projection, input.StatusFilter)
		threshold := issuemapDefaultThreshold(input.EdgeThreshold)
		return issuemap.BuildMapFromProjection(projection, input.Viewport, threshold)
	}
	storeIssues, err := h.IssueStore.List(ctx)
	if err != nil {
		return issuemap.MapResponse{}, err
	}
	storeIssues = issueviews.FilterIssuesByStatus(storeIssues, input.StatusFilter)

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
