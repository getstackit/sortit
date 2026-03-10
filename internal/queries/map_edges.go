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
	Corpus     *DerivedCorpusLoader
}

func (h EdgeHandler) Handle(ctx context.Context, input MapQuery) (issuemap.EdgeResponse, error) {
	if h.Corpus != nil {
		corpus, err := h.Corpus.Current(ctx)
		if err != nil {
			return issuemap.EdgeResponse{}, err
		}
		filtered := FilterIssuesByStatus(corpus.Issues, input.StatusFilter)
		corpus = subsetCorpusByIssues(corpus, filtered)
		threshold := issuemapDefaultThreshold(input.EdgeThreshold)
		return issuemap.BuildEdgeResponseFromCorpus(corpus, input.Viewport, threshold)
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
