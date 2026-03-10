package queries

import (
	"context"

	"splat/internal/issues"
	issuemap "splat/internal/map"
	"splat/internal/services"
)

type ExploreIssue struct {
	ID    string
	Limit int
}

type ExploreIssueHandler struct {
	Store   issues.Store
	Catalog *services.CatalogService
	Corpus  *DerivedCorpusLoader
}

func (h ExploreIssueHandler) Handle(ctx context.Context, input ExploreIssue) (issuemap.ExploreResponse, error) {
	if h.Corpus != nil {
		corpus, err := h.Corpus.Current(ctx)
		if err != nil {
			return issuemap.ExploreResponse{}, err
		}
		return issuemap.ExploreFromCorpus(corpus, input.ID, input.Limit)
	}
	storeIssues, err := h.Store.List(ctx)
	if err != nil {
		return issuemap.ExploreResponse{}, err
	}

	storeTags, err := h.Catalog.StoredTags(ctx)
	if err != nil {
		return issuemap.ExploreResponse{}, err
	}

	return issuemap.ExploreFromIssuesWithTags(storeIssues, storeTags, input.ID, input.Limit)
}
