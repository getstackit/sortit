package queries

import (
	"context"
	"sync"

	"splat/internal/issues"
	issuemap "splat/internal/map"
	"splat/internal/services"
)

type revisionSource interface {
	Revision() uint64
}

type ReadModel struct {
	Revision uint64
	Corpus   issuemap.DerivedCorpus
}

type ReadModelLoader struct {
	Store     issues.Store
	Catalog   *services.CatalogService
	Revisions revisionSource

	mu     sync.RWMutex
	cached *ReadModel
}

func (l *ReadModelLoader) Current(ctx context.Context) (ReadModel, error) {
	if l == nil {
		return ReadModel{}, nil
	}

	revision := uint64(0)
	if l.Revisions != nil {
		revision = l.Revisions.Revision()
	}

	l.mu.RLock()
	cached := l.cached
	if cached != nil && cached.Revision == revision {
		out := *cached
		l.mu.RUnlock()
		return out, nil
	}
	l.mu.RUnlock()

	model, err := l.rebuild(ctx, revision)
	if err != nil {
		return ReadModel{}, err
	}

	l.mu.Lock()
	l.cached = &model
	l.mu.Unlock()
	return model, nil
}

func (l *ReadModelLoader) rebuild(ctx context.Context, revision uint64) (ReadModel, error) {
	items, err := l.Store.List(ctx)
	if err != nil {
		return ReadModel{}, err
	}

	detailed := make([]issues.Issue, 0, len(items))
	for _, item := range items {
		issue, err := l.Store.Get(ctx, item.ID)
		if err != nil {
			return ReadModel{}, err
		}
		detailed = append(detailed, issue)
	}

	tags, err := l.Catalog.StoredTags(ctx)
	if err != nil {
		return ReadModel{}, err
	}

	corpus, err := issuemap.BuildDerivedCorpus(detailed, tags)
	if err != nil {
		return ReadModel{}, err
	}

	return ReadModel{
		Revision: revision,
		Corpus:   corpus,
	}, nil
}
