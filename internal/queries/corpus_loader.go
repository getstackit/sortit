package queries

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"splat/internal/issues"
	issuemap "splat/internal/map"
	"splat/internal/services"
)

type revisionSource interface {
	Revision() uint64
}

type DerivedCorpusLoader struct {
	Store       issues.Store
	Catalog     *services.CatalogService
	Revisions   revisionSource
	Projections issues.DerivedCorpusProjectionStore

	mu sync.Mutex
}

func (l *DerivedCorpusLoader) Current(ctx context.Context) (issuemap.DerivedCorpus, error) {
	if l == nil {
		return issuemap.DerivedCorpus{}, nil
	}

	revision := uint64(0)
	if l.Revisions != nil {
		revision = l.Revisions.Revision()
	}

	if corpus, err := l.loadProjection(ctx, revision); err == nil {
		return corpus, nil
	} else if !errors.Is(err, issues.ErrDerivedCorpusNotFound) {
		return issuemap.DerivedCorpus{}, err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if corpus, err := l.loadProjection(ctx, revision); err == nil {
		return corpus, nil
	} else if !errors.Is(err, issues.ErrDerivedCorpusNotFound) {
		return issuemap.DerivedCorpus{}, err
	}

	corpus, err := l.rebuild(ctx)
	if err != nil {
		return issuemap.DerivedCorpus{}, err
	}

	if l.Projections != nil {
		payload, err := json.Marshal(corpus)
		if err != nil {
			return issuemap.DerivedCorpus{}, fmt.Errorf("marshal derived corpus: %w", err)
		}
		if err := l.Projections.SaveDerivedCorpusProjection(ctx, revision, payload); err != nil {
			return issuemap.DerivedCorpus{}, err
		}
	}

	return corpus, nil
}

func (l *DerivedCorpusLoader) loadProjection(ctx context.Context, revision uint64) (issuemap.DerivedCorpus, error) {
	if l == nil || l.Projections == nil {
		return issuemap.DerivedCorpus{}, issues.ErrDerivedCorpusNotFound
	}

	payload, err := l.Projections.GetDerivedCorpusProjection(ctx, revision)
	if err != nil {
		return issuemap.DerivedCorpus{}, err
	}

	var corpus issuemap.DerivedCorpus
	if err := json.Unmarshal(payload, &corpus); err != nil {
		return issuemap.DerivedCorpus{}, fmt.Errorf("decode derived corpus projection: %w", err)
	}
	return corpus, nil
}

func (l *DerivedCorpusLoader) rebuild(ctx context.Context) (issuemap.DerivedCorpus, error) {
	items, tags, err := loadDetailedIssuesAndTags(ctx, l.Store, l.Catalog)
	if err != nil {
		return issuemap.DerivedCorpus{}, err
	}
	return issuemap.BuildDerivedCorpus(items, tags)
}

func loadDetailedIssuesAndTags(ctx context.Context, store issues.Store, catalog *services.CatalogService) ([]issues.Issue, []issues.Tag, error) {
	items, err := store.List(ctx)
	if err != nil {
		return nil, nil, err
	}

	detailed := make([]issues.Issue, 0, len(items))
	for _, item := range items {
		issue, err := store.Get(ctx, item.ID)
		if err != nil {
			return nil, nil, err
		}
		detailed = append(detailed, issue)
	}

	tags, err := catalog.StoredTags(ctx)
	if err != nil {
		return nil, nil, err
	}

	return detailed, tags, nil
}
