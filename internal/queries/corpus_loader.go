package queries

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

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

type DerivedCorpusLoadStep struct {
	Name     string
	Duration time.Duration
}

type DetailedIssueLoadProfile struct {
	ListedIssueCount   int
	DetailedIssueCount int
	StoredTagCount     int
	Steps              []DerivedCorpusLoadStep
	TotalDuration      time.Duration
}

type DerivedCorpusLoadProfile struct {
	Revision        uint64
	CacheHit        bool
	ProjectionSaved bool
	DetailedLoad    DetailedIssueLoadProfile
	Build           issuemap.BuildDerivedCorpusProfile
	Steps           []DerivedCorpusLoadStep
	TotalDuration   time.Duration
}

func (l *DerivedCorpusLoader) Current(ctx context.Context) (issuemap.DerivedCorpus, error) {
	corpus, _, err := l.current(ctx, false)
	return corpus, err
}

func (l *DerivedCorpusLoader) ProfileCurrent(ctx context.Context) (issuemap.DerivedCorpus, DerivedCorpusLoadProfile, error) {
	return l.current(ctx, true)
}

func (l *DerivedCorpusLoader) current(ctx context.Context, captureProfile bool) (issuemap.DerivedCorpus, DerivedCorpusLoadProfile, error) {
	if l == nil {
		return issuemap.DerivedCorpus{}, DerivedCorpusLoadProfile{}, nil
	}

	startedAt := time.Now()
	profile := DerivedCorpusLoadProfile{}
	revision := uint64(0)
	if l.Revisions != nil {
		revision = l.Revisions.Revision()
	}
	profile.Revision = revision

	stepStartedAt := time.Now()
	if corpus, err := l.loadProjection(ctx, revision); err == nil {
		if captureProfile {
			profile.CacheHit = true
			profile.Steps = append(profile.Steps, DerivedCorpusLoadStep{
				Name:     "load_projection_initial",
				Duration: time.Since(stepStartedAt),
			})
			profile.TotalDuration = time.Since(startedAt)
		}
		return corpus, profile, nil
	} else if !errors.Is(err, issues.ErrDerivedCorpusNotFound) {
		return issuemap.DerivedCorpus{}, DerivedCorpusLoadProfile{}, err
	} else if captureProfile {
		profile.Steps = append(profile.Steps, DerivedCorpusLoadStep{
			Name:     "load_projection_initial",
			Duration: time.Since(stepStartedAt),
		})
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	stepStartedAt = time.Now()
	if corpus, err := l.loadProjection(ctx, revision); err == nil {
		if captureProfile {
			profile.CacheHit = true
			profile.Steps = append(profile.Steps, DerivedCorpusLoadStep{
				Name:     "load_projection_locked",
				Duration: time.Since(stepStartedAt),
			})
			profile.TotalDuration = time.Since(startedAt)
		}
		return corpus, profile, nil
	} else if !errors.Is(err, issues.ErrDerivedCorpusNotFound) {
		return issuemap.DerivedCorpus{}, DerivedCorpusLoadProfile{}, err
	} else if captureProfile {
		profile.Steps = append(profile.Steps, DerivedCorpusLoadStep{
			Name:     "load_projection_locked",
			Duration: time.Since(stepStartedAt),
		})
	}

	corpus, rebuildProfile, err := l.rebuildProfiled(ctx)
	if err != nil {
		return issuemap.DerivedCorpus{}, DerivedCorpusLoadProfile{}, err
	}
	if captureProfile {
		profile.DetailedLoad = rebuildProfile.load
		profile.Build = rebuildProfile.build
		profile.Steps = append(profile.Steps, DerivedCorpusLoadStep{
			Name:     "rebuild_total",
			Duration: rebuildProfile.total,
		})
	}

	if l.Projections != nil {
		stepStartedAt = time.Now()
		payload, err := json.Marshal(corpus)
		if err != nil {
			return issuemap.DerivedCorpus{}, DerivedCorpusLoadProfile{}, fmt.Errorf("marshal derived corpus: %w", err)
		}
		if captureProfile {
			profile.Steps = append(profile.Steps, DerivedCorpusLoadStep{
				Name:     "marshal_projection",
				Duration: time.Since(stepStartedAt),
			})
		}

		stepStartedAt = time.Now()
		if err := l.Projections.SaveDerivedCorpusProjection(ctx, revision, payload); err != nil {
			return issuemap.DerivedCorpus{}, DerivedCorpusLoadProfile{}, err
		}
		if captureProfile {
			profile.ProjectionSaved = true
			profile.Steps = append(profile.Steps, DerivedCorpusLoadStep{
				Name:     "save_projection",
				Duration: time.Since(stepStartedAt),
			})
		}
	}

	if captureProfile {
		profile.TotalDuration = time.Since(startedAt)
	}
	return corpus, profile, nil
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
	corpus, _, err := l.rebuildProfiled(ctx)
	return corpus, err
}

type rebuildProfile struct {
	load  DetailedIssueLoadProfile
	build issuemap.BuildDerivedCorpusProfile
	total time.Duration
}

func (l *DerivedCorpusLoader) rebuildProfiled(ctx context.Context) (issuemap.DerivedCorpus, rebuildProfile, error) {
	startedAt := time.Now()
	items, tags, loadProfile, err := loadDetailedIssuesAndTagsProfiled(ctx, l.Store, l.Catalog)
	if err != nil {
		return issuemap.DerivedCorpus{}, rebuildProfile{}, err
	}

	corpus, buildProfile, err := issuemap.BuildDerivedCorpusProfiled(items, tags)
	if err != nil {
		return issuemap.DerivedCorpus{}, rebuildProfile{}, err
	}

	return corpus, rebuildProfile{
		load:  loadProfile,
		build: buildProfile,
		total: time.Since(startedAt),
	}, nil
}

func loadDetailedIssuesAndTags(ctx context.Context, store issues.Store, catalog *services.CatalogService) ([]issues.Issue, []issues.Tag, error) {
	items, tags, _, err := loadDetailedIssuesAndTagsProfiled(ctx, store, catalog)
	return items, tags, err
}

func loadDetailedIssuesAndTagsProfiled(
	ctx context.Context,
	store issues.Store,
	catalog *services.CatalogService,
) ([]issues.Issue, []issues.Tag, DetailedIssueLoadProfile, error) {
	startedAt := time.Now()
	profile := DetailedIssueLoadProfile{}

	stepStartedAt := time.Now()
	items, err := store.List(ctx)
	if err != nil {
		return nil, nil, DetailedIssueLoadProfile{}, err
	}
	profile.ListedIssueCount = len(items)
	profile.Steps = append(profile.Steps, DerivedCorpusLoadStep{
		Name:     "store_list",
		Duration: time.Since(stepStartedAt),
	})

	detailed := make([]issues.Issue, 0, len(items))
	stepStartedAt = time.Now()
	for _, item := range items {
		issue, err := store.Get(ctx, item.ID)
		if err != nil {
			return nil, nil, DetailedIssueLoadProfile{}, err
		}
		detailed = append(detailed, issue)
	}
	profile.DetailedIssueCount = len(detailed)
	profile.Steps = append(profile.Steps, DerivedCorpusLoadStep{
		Name:     "store_get_details",
		Duration: time.Since(stepStartedAt),
	})

	stepStartedAt = time.Now()
	tags, err := catalog.StoredTags(ctx)
	if err != nil {
		return nil, nil, DetailedIssueLoadProfile{}, err
	}
	profile.StoredTagCount = len(tags)
	profile.Steps = append(profile.Steps, DerivedCorpusLoadStep{
		Name:     "load_stored_tags",
		Duration: time.Since(stepStartedAt),
	})
	profile.TotalDuration = time.Since(startedAt)

	return detailed, tags, profile, nil
}
