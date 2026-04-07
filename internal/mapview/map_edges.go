package mapview

import (
	"context"

	"splat/internal/issues"
	issueviews "splat/internal/issues/views"
	issuemap "splat/internal/map"
	"splat/internal/tags"
)

type EdgeHandler struct {
	IssueStore issues.Store
	Catalog    *tags.CatalogService
	Projection *MapProjectionLoader
}

func (h EdgeHandler) Handle(ctx context.Context, input MapQuery) (issuemap.EdgeResponse, error) {
	if h.Projection != nil {
		projection, err := h.Projection.Current(ctx)
		if err != nil {
			return issuemap.EdgeResponse{}, err
		}
		storeIssues, err := issueviews.ListProjectionIssueMetadata(ctx, h.IssueStore)
		if err != nil {
			return issuemap.EdgeResponse{}, err
		}
		projection = issueviews.OverlayProjectionIssueMetadata(projection, storeIssues)
		projection = issueviews.SubsetProjectionByStatus(projection, input.StatusFilter)
		threshold := issuemapDefaultThreshold(input.EdgeThreshold)
		return issuemap.BuildEdgeResponseFromProjection(projection, input.Viewport, threshold)
	}
	storeIssues, err := h.IssueStore.List(ctx)
	if err != nil {
		return issuemap.EdgeResponse{}, err
	}
	storeIssues = issueviews.FilterIssuesByStatus(storeIssues, input.StatusFilter)

	storeTags, err := h.Catalog.StoredTags(ctx)
	if err != nil {
		return issuemap.EdgeResponse{}, err
	}

	if input.EdgeThreshold != nil {
		return issuemap.BuildEdgeResponseFromIssuesWithTagsAndThreshold(storeIssues, storeTags, input.Viewport, *input.EdgeThreshold)
	}
	return issuemap.BuildEdgeResponseFromIssuesWithTags(storeIssues, storeTags, input.Viewport)
}
