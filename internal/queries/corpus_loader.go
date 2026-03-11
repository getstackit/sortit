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

type MapProjectionLoader struct {
	Store       issues.Store
	Catalog     *services.CatalogService
	Revisions   revisionSource
	Projections issues.MapProjectionStorePersistence

	mu sync.Mutex
}

type MapProjectionLoadStep struct {
	Name     string
	Duration time.Duration
}

type DetailedIssueLoadProfile struct {
	ListedIssueCount   int
	DetailedIssueCount int
	StoredTagCount     int
	Steps              []MapProjectionLoadStep
	TotalDuration      time.Duration
}

type MapProjectionLoadProfile struct {
	Revision        uint64
	CacheHit        bool
	ProjectionSaved bool
	DetailedLoad    DetailedIssueLoadProfile
	Build           issuemap.BuildMapProjectionProfile
	Steps           []MapProjectionLoadStep
	TotalDuration   time.Duration
}

func (l *MapProjectionLoader) Current(ctx context.Context) (issuemap.MapProjection, error) {
	projection, _, err := l.current(ctx, false)
	return projection, err
}

func (l *MapProjectionLoader) ProfileCurrent(ctx context.Context) (issuemap.MapProjection, MapProjectionLoadProfile, error) {
	return l.current(ctx, true)
}

func (l *MapProjectionLoader) current(ctx context.Context, captureProfile bool) (issuemap.MapProjection, MapProjectionLoadProfile, error) {
	if l == nil {
		return issuemap.MapProjection{}, MapProjectionLoadProfile{}, nil
	}

	startedAt := time.Now()
	profile := MapProjectionLoadProfile{}
	revision := uint64(0)
	if l.Revisions != nil {
		revision = l.Revisions.Revision()
	}
	profile.Revision = revision

	stepStartedAt := time.Now()
	if projection, err := l.loadProjection(ctx, revision); err == nil {
		if captureProfile {
			profile.CacheHit = true
			profile.Steps = append(profile.Steps, MapProjectionLoadStep{
				Name:     "load_projection_initial",
				Duration: time.Since(stepStartedAt),
			})
			profile.TotalDuration = time.Since(startedAt)
		}
		return projection, profile, nil
	} else if !errors.Is(err, issues.ErrMapProjectionNotFound) {
		return issuemap.MapProjection{}, MapProjectionLoadProfile{}, err
	} else if captureProfile {
		profile.Steps = append(profile.Steps, MapProjectionLoadStep{
			Name:     "load_projection_initial",
			Duration: time.Since(stepStartedAt),
		})
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	stepStartedAt = time.Now()
	if projection, err := l.loadProjection(ctx, revision); err == nil {
		if captureProfile {
			profile.CacheHit = true
			profile.Steps = append(profile.Steps, MapProjectionLoadStep{
				Name:     "load_projection_locked",
				Duration: time.Since(stepStartedAt),
			})
			profile.TotalDuration = time.Since(startedAt)
		}
		return projection, profile, nil
	} else if !errors.Is(err, issues.ErrMapProjectionNotFound) {
		return issuemap.MapProjection{}, MapProjectionLoadProfile{}, err
	} else if captureProfile {
		profile.Steps = append(profile.Steps, MapProjectionLoadStep{
			Name:     "load_projection_locked",
			Duration: time.Since(stepStartedAt),
		})
	}

	projection, rebuildProfile, err := l.rebuildProfiled(ctx)
	if err != nil {
		return issuemap.MapProjection{}, MapProjectionLoadProfile{}, err
	}
	if captureProfile {
		profile.DetailedLoad = rebuildProfile.load
		profile.Build = rebuildProfile.build
		profile.Steps = append(profile.Steps, MapProjectionLoadStep{
			Name:     "rebuild_total",
			Duration: rebuildProfile.total,
		})
	}

	if l.Projections != nil {
		stepStartedAt = time.Now()
		payload, err := json.Marshal(projection)
		if err != nil {
			return issuemap.MapProjection{}, MapProjectionLoadProfile{}, fmt.Errorf("marshal map projection: %w", err)
		}
		if captureProfile {
			profile.Steps = append(profile.Steps, MapProjectionLoadStep{
				Name:     "marshal_projection",
				Duration: time.Since(stepStartedAt),
			})
		}

		stepStartedAt = time.Now()
		if err := l.Projections.SaveMapProjection(ctx, revision, payload); err != nil {
			return issuemap.MapProjection{}, MapProjectionLoadProfile{}, err
		}
		if captureProfile {
			profile.ProjectionSaved = true
			profile.Steps = append(profile.Steps, MapProjectionLoadStep{
				Name:     "save_projection",
				Duration: time.Since(stepStartedAt),
			})
		}
	}

	if captureProfile {
		profile.TotalDuration = time.Since(startedAt)
	}
	return projection, profile, nil
}

func (l *MapProjectionLoader) loadProjection(ctx context.Context, revision uint64) (issuemap.MapProjection, error) {
	if l == nil || l.Projections == nil {
		return issuemap.MapProjection{}, issues.ErrMapProjectionNotFound
	}

	payload, err := l.Projections.GetMapProjection(ctx, revision)
	if err != nil {
		return issuemap.MapProjection{}, err
	}

	var projection issuemap.MapProjection
	if err := json.Unmarshal(payload, &projection); err != nil {
		return issuemap.MapProjection{}, fmt.Errorf("decode map projection: %w", err)
	}
	return projection, nil
}

func (l *MapProjectionLoader) rebuild(ctx context.Context) (issuemap.MapProjection, error) {
	projection, _, err := l.rebuildProfiled(ctx)
	return projection, err
}

type rebuildProfile struct {
	load  DetailedIssueLoadProfile
	build issuemap.BuildMapProjectionProfile
	total time.Duration
}

func (l *MapProjectionLoader) rebuildProfiled(ctx context.Context) (issuemap.MapProjection, rebuildProfile, error) {
	startedAt := time.Now()
	items, tags, loadProfile, err := loadDetailedIssuesAndTagsProfiled(ctx, l.Store, l.Catalog)
	if err != nil {
		return issuemap.MapProjection{}, rebuildProfile{}, err
	}

	projection, buildProfile, err := issuemap.BuildMapProjectionProfiled(items, tags)
	if err != nil {
		return issuemap.MapProjection{}, rebuildProfile{}, err
	}

	return projection, rebuildProfile{
		load:  loadProfile,
		build: buildProfile,
		total: time.Since(startedAt),
	}, nil
}

func loadDetailedIssuesAndTags(
	ctx context.Context,
	store issues.Store,
	catalog *services.CatalogService,
) ([]issues.MapProjectionIssue, []issues.Tag, error) {
	items, tags, _, err := loadDetailedIssuesAndTagsProfiled(ctx, store, catalog)
	return items, tags, err
}

func loadDetailedIssuesAndTagsProfiled(
	ctx context.Context,
	store issues.Store,
	catalog *services.CatalogService,
) ([]issues.MapProjectionIssue, []issues.Tag, DetailedIssueLoadProfile, error) {
	startedAt := time.Now()
	profile := DetailedIssueLoadProfile{}

	if projectionStore, ok := store.(issues.MapProjectionStore); ok {
		stepStartedAt := time.Now()
		items, tags, err := projectionStore.LoadMapProjectionData(ctx)
		if err != nil {
			return nil, nil, DetailedIssueLoadProfile{}, err
		}
		profile.ListedIssueCount = len(items)
		profile.DetailedIssueCount = len(items)
		profile.StoredTagCount = len(tags)
		profile.Steps = append(profile.Steps, MapProjectionLoadStep{
			Name:     "load_map_projection_data",
			Duration: time.Since(stepStartedAt),
		})
		profile.TotalDuration = time.Since(startedAt)
		return items, tags, profile, nil
	}

	stepStartedAt := time.Now()
	items, err := store.List(ctx)
	if err != nil {
		return nil, nil, DetailedIssueLoadProfile{}, err
	}
	profile.ListedIssueCount = len(items)
	profile.Steps = append(profile.Steps, MapProjectionLoadStep{
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
	profile.Steps = append(profile.Steps, MapProjectionLoadStep{
		Name:     "store_get_details",
		Duration: time.Since(stepStartedAt),
	})

	stepStartedAt = time.Now()
	tags, err := catalog.StoredTags(ctx)
	if err != nil {
		return nil, nil, DetailedIssueLoadProfile{}, err
	}
	profile.StoredTagCount = len(tags)
	profile.Steps = append(profile.Steps, MapProjectionLoadStep{
		Name:     "load_stored_tags",
		Duration: time.Since(stepStartedAt),
	})
	profile.TotalDuration = time.Since(startedAt)

	return issues.MapProjectionIssuesFromIssues(detailed), tags, profile, nil
}
