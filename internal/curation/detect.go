package curation

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"

	"sortit/internal/issues"
	issueviews "sortit/internal/issues/views"
	issuemap "sortit/internal/map"
	"sortit/internal/mapview"
)

// Detector computes read-only curation candidates the librarian agent reads,
// judges (with codebase context), and turns into proposals. Detection never
// writes proposals or mutates state — it only surfaces what may be worth a move.
type Detector struct {
	reader   issues.Reader
	detail   issues.IssueDetailReader
	explorer IssueExplorer
	logger   *slog.Logger
}

// IssueExplorer is the similarity surface duplicate detection rides on.
// mapview.ExploreIssueHandler satisfies it.
type IssueExplorer interface {
	Handle(ctx context.Context, in mapview.ExploreIssue) (issuemap.ExploreResponse, error)
}

// NewDetector constructs a candidate detector. detail may be nil (stale
// detection then falls back to whatever metrics List already carries).
func NewDetector(reader issues.Reader, detail issues.IssueDetailReader, explorer IssueExplorer, logger *slog.Logger) *Detector {
	if logger == nil {
		logger = slog.Default()
	}
	return &Detector{reader: reader, detail: detail, explorer: explorer, logger: logger.With("component", "curation.detect")}
}

// --- Duplicate consolidation ---

// DuplicateCluster groups open issues that are highly similar and may be worth
// combining. Similarity is the minimum edge similarity that bound the cluster
// together (a conservative cohesion floor). SuggestedCanonical is advisory — the
// combine handler always mints a new combined issue.
type DuplicateCluster struct {
	IssueIDs           []string `json:"issueIds"`
	Similarity         float64  `json:"similarity"`
	SharedTags         []string `json:"sharedTags,omitempty"`
	SuggestedCanonical string   `json:"suggestedCanonical,omitempty"`
}

// DuplicateParams tunes duplicate detection. Zero values fall back to defaults.
type DuplicateParams struct {
	MinSimilarity float64 // edge threshold to link two issues (default 0.85)
	MaxSeeds      int     // cap seed issues explored; 0 = all open issues
	ExploreLimit  int     // per-seed related-issue limit (default 10)
	Limit         int     // max clusters returned; 0 = all
}

func (p DuplicateParams) withDefaults() DuplicateParams {
	if p.MinSimilarity <= 0 {
		p.MinSimilarity = 0.85
	}
	if p.ExploreLimit <= 0 {
		p.ExploreLimit = 10
	}
	return p
}

// DetectDuplicates explores each open issue's neighborhood and unions pairs that
// exceed the similarity threshold into clusters. It runs one explore per seed
// (which recomputes corpus decompositions), so it is agent-triggered, not a hot
// path — bound the work with MaxSeeds/Limit on large corpora.
func (d *Detector) DetectDuplicates(ctx context.Context, params DuplicateParams) ([]DuplicateCluster, error) {
	params = params.withDefaults()

	all, err := d.reader.List(ctx)
	if err != nil {
		return nil, err
	}
	open := make(map[string]issues.Issue, len(all))
	order := make([]issues.Issue, 0, len(all))
	for _, issue := range all {
		if issue.Status == issues.StatusOpen {
			open[issue.ID] = issue
			order = append(order, issue)
		}
	}

	// Seed from the most recently created open issues first so a capped sweep
	// covers the freshest (most likely duplicated) work.
	sort.SliceStable(order, func(i, j int) bool { return order[i].CreatedAt.After(order[j].CreatedAt) })
	if params.MaxSeeds > 0 && len(order) > params.MaxSeeds {
		order = order[:params.MaxSeeds]
	}

	uf := newUnionFind()
	edgeSim := map[string]float64{} // member id -> min incident edge similarity
	noteEdge := func(id string, sim float64) {
		if existing, ok := edgeSim[id]; !ok || sim < existing {
			edgeSim[id] = sim
		}
	}

	for _, seed := range order {
		resp, err := d.explorer.Handle(ctx, mapview.ExploreIssue{ID: seed.ID, Limit: params.ExploreLimit})
		if err != nil {
			d.logger.WarnContext(ctx, "explore failed during duplicate detection", "issue_id", seed.ID, "error", err)
			continue
		}
		for _, related := range resp.RelatedIssues {
			if related.Status != issues.StatusOpen {
				continue
			}
			if _, ok := open[related.ID]; !ok {
				continue
			}
			if related.CombinedSimilarity < params.MinSimilarity {
				continue
			}
			uf.union(seed.ID, related.ID)
			noteEdge(seed.ID, related.CombinedSimilarity)
			noteEdge(related.ID, related.CombinedSimilarity)
		}
	}

	clusters := make([]DuplicateCluster, 0)
	for _, members := range uf.groups() {
		if len(members) < 2 {
			continue
		}
		memberIssues := make([]issues.Issue, 0, len(members))
		minSim := 1.0
		for _, id := range members {
			memberIssues = append(memberIssues, open[id])
			if sim, ok := edgeSim[id]; ok && sim < minSim {
				minSim = sim
			}
		}
		sort.Strings(members)
		clusters = append(clusters, DuplicateCluster{
			IssueIDs:           members,
			Similarity:         round3(minSim),
			SharedTags:         sharedTags(memberIssues),
			SuggestedCanonical: oldestIssueID(memberIssues),
		})
	}

	sort.SliceStable(clusters, func(i, j int) bool { return clusters[i].Similarity > clusters[j].Similarity })
	if params.Limit > 0 && len(clusters) > params.Limit {
		clusters = clusters[:params.Limit]
	}
	return clusters, nil
}

// --- Weeding (stale issues) ---

// StaleIssue is a long-inactive open issue that may be worth closing as stale.
type StaleIssue struct {
	IssueID        string    `json:"issueId"`
	Summary        string    `json:"summary"`
	LastActivityAt time.Time `json:"lastActivityAt"`
	DaysInactive   float64   `json:"daysInactive"`
	Velocity       *float64  `json:"velocity,omitempty"`
}

// StaleParams tunes stale detection. Zero values fall back to defaults.
type StaleParams struct {
	MinDaysInactive float64 // default 90
	MaxVelocity     float64 // default 0.05; issues above this are still active
	Limit           int     // max results; 0 = all
}

func (p StaleParams) withDefaults() StaleParams {
	if p.MinDaysInactive <= 0 {
		p.MinDaysInactive = 90
	}
	if p.MaxVelocity <= 0 {
		p.MaxVelocity = 0.05
	}
	return p
}

// DetectStaleIssues flags open issues whose last activity is older than
// MinDaysInactive and whose velocity is at or below MaxVelocity. It hydrates
// each issue for its discussion + velocity, so it is agent-triggered, not a hot
// path.
func (d *Detector) DetectStaleIssues(ctx context.Context, params StaleParams) ([]StaleIssue, error) {
	params = params.withDefaults()
	now := time.Now().UTC()

	all, err := d.reader.List(ctx)
	if err != nil {
		return nil, err
	}

	stale := make([]StaleIssue, 0)
	for _, issue := range all {
		if issue.Status != issues.StatusOpen {
			continue
		}
		hydrated := issueviews.HydrateIssueWithVelocity(ctx, d.detail, issue, now)
		last := lastActivityAt(hydrated)
		daysInactive := now.Sub(last).Hours() / 24
		if daysInactive < params.MinDaysInactive {
			continue
		}
		velocity := velocityOf(hydrated)
		if velocity != nil && *velocity > params.MaxVelocity {
			continue
		}
		stale = append(stale, StaleIssue{
			IssueID:        hydrated.ID,
			Summary:        firstLine(hydrated.Raw),
			LastActivityAt: last,
			DaysInactive:   round1(daysInactive),
			Velocity:       velocity,
		})
	}

	sort.SliceStable(stale, func(i, j int) bool { return stale[i].DaysInactive > stale[j].DaysInactive })
	if params.Limit > 0 && len(stale) > params.Limit {
		stale = stale[:params.Limit]
	}
	return stale, nil
}

// --- helpers ---

func lastActivityAt(issue issues.Issue) time.Time {
	last := issue.CreatedAt
	for _, post := range issue.Discussion {
		if post.CreatedAt.After(last) {
			last = post.CreatedAt
		}
	}
	return last
}

func velocityOf(issue issues.Issue) *float64 {
	if issue.LifecycleMetrics == nil || issue.LifecycleMetrics.Velocity == nil {
		return nil
	}
	v := *issue.LifecycleMetrics.Velocity
	return &v
}

// sharedTags returns tags present on every member of the cluster.
func sharedTags(members []issues.Issue) []string {
	if len(members) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, m := range members {
		seen := map[string]struct{}{}
		for _, tag := range m.Tags {
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}
			counts[tag]++
		}
	}
	out := make([]string, 0)
	for tag, n := range counts {
		if n == len(members) {
			out = append(out, tag)
		}
	}
	sort.Strings(out)
	return out
}

func oldestIssueID(members []issues.Issue) string {
	if len(members) == 0 {
		return ""
	}
	oldest := members[0]
	for _, m := range members[1:] {
		if m.CreatedAt.Before(oldest.CreatedAt) {
			oldest = m
		}
	}
	return oldest.ID
}

func firstLine(raw string) string {
	raw = strings.TrimSpace(raw)
	if idx := strings.IndexByte(raw, '\n'); idx >= 0 {
		raw = strings.TrimSpace(raw[:idx])
	}
	const maxLen = 120
	if len(raw) > maxLen {
		raw = strings.TrimSpace(raw[:maxLen]) + "…"
	}
	return raw
}

func round1(v float64) float64 { return float64(int64(v*10+0.5)) / 10 }
func round3(v float64) float64 { return float64(int64(v*1000+0.5)) / 1000 }

// unionFind is a small disjoint-set over issue ids for clustering.
type unionFind struct {
	parent map[string]string
}

func newUnionFind() *unionFind { return &unionFind{parent: map[string]string{}} }

func (u *unionFind) find(x string) string {
	if _, ok := u.parent[x]; !ok {
		u.parent[x] = x
		return x
	}
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(a, b string) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[ra] = rb
	}
}

func (u *unionFind) groups() map[string][]string {
	out := map[string][]string{}
	for node := range u.parent {
		root := u.find(node)
		out[root] = append(out[root], node)
	}
	return out
}
