package queries

import (
	"context"

	"splat/internal/issues"
	issuemap "splat/internal/map"
	"splat/internal/services"
)

type EdgeHandler struct {
	IssueStore issues.Store
	Catalog    *services.CatalogService
	Projection *MapProjectionLoader
}

func (h EdgeHandler) Handle(ctx context.Context, input MapQuery) (issuemap.EdgeResponse, error) {
	if h.Projection != nil {
		projection, err := h.Projection.Current(ctx)
		if err != nil {
			return issuemap.EdgeResponse{}, err
		}
		projection = subsetProjectionByStatus(projection, input.StatusFilter)
		threshold := issuemapDefaultThreshold(input.EdgeThreshold)
		return issuemap.BuildEdgeResponseFromProjection(projection, input.Viewport, threshold)
	}
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
