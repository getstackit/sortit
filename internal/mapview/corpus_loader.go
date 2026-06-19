package mapview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
	issuemap "sortit/internal/map"
	"sortit/internal/tags"
)

type revisionSource interface {
	Revision() uint64
}

type MapProjectionLoader struct {
	Store       detailedIssueReader
	Catalog     *tags.CatalogService
	Revisions   revisionSource
	Projections issues.MapProjectionStorePersistence
	// Memories, when set, overlays permanent memories as map landmarks. It is
	// optional: a nil store simply yields no landmarks.
	Memories issues.MemoryStore

	mu sync.Mutex

	// prevPositions is the layout from the most recent projection this
	// loader produced or loaded, regardless of revision. It seeds the
	// Procrustes alignment on the next rebuild so a revision bump doesn't
	// reorient the map (in-memory only, mirroring the cache pattern in
	// internal/tagcooccurrence: after a cold start the first rebuild has
	// no reference and orientation falls back to the deterministic
	// loading-sign convention).
	prevMu        sync.Mutex
	prevPositions map[string]issuemap.Position
}

// rememberLayout stashes a projection's positions as the alignment
// reference for the next rebuild.
func (l *MapProjectionLoader) rememberLayout(projection issuemap.MapProjection) {
	if len(projection.MapIssues) == 0 {
		return
	}
	positions := make(map[string]issuemap.Position, len(projection.MapIssues))
	for _, item := range projection.MapIssues {
		positions[item.ID] = issuemap.Position{X: item.X, Y: item.Y}
	}
	l.prevMu.Lock()
	l.prevPositions = positions
	l.prevMu.Unlock()
}

func (l *MapProjectionLoader) previousLayout() map[string]issuemap.Position {
	l.prevMu.Lock()
	defer l.prevMu.Unlock()
	return l.prevPositions
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

type detailedIssueReader interface {
	List(context.Context) ([]issues.Issue, error)
	GetIssueDetail(context.Context, string) (issues.Issue, error)
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
		l.rememberLayout(projection)
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
		l.rememberLayout(projection)
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
	l.rememberLayout(projection)
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

	projection, buildProfile, err := issuemap.BuildMapProjectionProfiledAligned(items, tags, l.previousLayout())
	if err != nil {
		return issuemap.MapProjection{}, rebuildProfile{}, err
	}

	if err := l.attachLandmarks(ctx, &projection, items); err != nil {
		return issuemap.MapProjection{}, rebuildProfile{}, err
	}

	return projection, rebuildProfile{
		load:  loadProfile,
		build: buildProfile,
		total: time.Since(startedAt),
	}, nil
}

// attachLandmarks overlays active memories onto the freshly built projection as
// map landmarks. Memories consume the issue-derived layout (positions and
// clusters) without being part of the PCA fit, so they never distort the model
// they were derived from.
func (l *MapProjectionLoader) attachLandmarks(ctx context.Context, projection *issuemap.MapProjection, items []issues.MapProjectionIssue) error {
	if l.Memories == nil || !projection.Available || len(projection.MapIssues) == 0 {
		return nil
	}

	mems, err := l.Memories.ListMemories(ctx, issues.MemoryListOptions{Status: domain.MemoryStatusActive})
	if err != nil {
		return err
	}
	if len(mems) == 0 {
		return nil
	}

	issuePositions := make(map[string]issuemap.Position, len(projection.MapIssues))
	for _, mapIssue := range projection.MapIssues {
		issuePositions[mapIssue.ID] = issuemap.Position{X: mapIssue.X, Y: mapIssue.Y}
	}
	issueEmbeddings := make(map[string][]float64, len(items))
	for _, item := range items {
		if len(item.Embedding) > 0 {
			issueEmbeddings[item.ID] = item.Embedding
		}
	}

	projection.Landmarks = issuemap.ComputeMemoryLandmarks(mems, issuePositions, issueEmbeddings, projection.Clusters)
	return nil
}

func loadDetailedIssuesAndTagsProfiled(
	ctx context.Context,
	store detailedIssueReader,
	catalog *tags.CatalogService,
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
		issue, err := store.GetIssueDetail(ctx, item.ID)
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
