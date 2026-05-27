package regions

import (
	"math"
	"sort"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/vectors"
)

// Age bucket labels in display order.
const (
	AgeBucketUnderOneWeek     = "<1w"
	AgeBucketOneToFourWeeks   = "1-4w"
	AgeBucketOneToThreeMonths = "1-3m"
	AgeBucketThreeMonthsPlus  = "3m+"
)

// RegionWithMetrics is the per-region payload returned by the metrics API.
type RegionWithMetrics struct {
	Region  domain.Region        `json:"region"`
	Metrics domain.RegionMetrics `json:"metrics"`
}

// ComputeMass returns the total, open, and closed issue counts for the
// region. Counts are derived from BelongsTo, so the membership semantics
// stay in one place.
func ComputeMass(items []issues.Issue, key domain.RegionKey) (mass, open, closed int) {
	for _, issue := range items {
		if !BelongsTo(issue, key) {
			continue
		}
		mass++
		if issue.Status == issues.StatusClosed {
			closed++
		} else {
			open++
		}
	}
	return mass, open, closed
}

// ComputeAgeBuckets counts currently-open issues in the region by age
// (now - CreatedAt) using the fixed bucket layout: <1w, 1-4w, 1-3m, 3m+.
// Boundaries are half-open: an issue exactly at the 7-day mark falls into
// the 1-4w bucket.
func ComputeAgeBuckets(items []issues.Issue, key domain.RegionKey, now time.Time) []domain.AgeBucket {
	buckets := []domain.AgeBucket{
		{Label: AgeBucketUnderOneWeek},
		{Label: AgeBucketOneToFourWeeks},
		{Label: AgeBucketOneToThreeMonths},
		{Label: AgeBucketThreeMonthsPlus},
	}
	for _, issue := range items {
		if issue.Status != issues.StatusOpen {
			continue
		}
		if !BelongsTo(issue, key) {
			continue
		}
		age := now.Sub(issue.CreatedAt)
		switch {
		case age < 7*24*time.Hour:
			buckets[0].Count++
		case age < 28*24*time.Hour:
			buckets[1].Count++
		case age < 90*24*time.Hour:
			buckets[2].Count++
		default:
			buckets[3].Count++
		}
	}
	return buckets
}

// ComputeGrowth counts currently-in-region issues whose CreatedAt falls
// in [window.Start, window.End], returning Count and PerDay. Returns nil
// when the window has no finite Start (label "all"), since PerDay has no
// meaningful denominator without a bounded span.
//
// Membership is current-state by design: an issue created in the window
// counts iff it is currently a member of the region. This matches the
// closed-factor-attribution timeline's precedent.
func ComputeGrowth(items []issues.Issue, key domain.RegionKey, window domain.TimeWindow) *domain.Rate {
	if window.Start.IsZero() {
		return nil
	}
	count := 0
	for _, issue := range items {
		if !BelongsTo(issue, key) {
			continue
		}
		if !inWindow(issue.CreatedAt, window) {
			continue
		}
		count++
	}
	return rateOver(count, window)
}

// ComputeClosure counts currently-in-region issues whose ClosedAt falls
// in window, restricted to Status == closed. Returns nil for the "all"
// window like ComputeGrowth.
func ComputeClosure(items []issues.Issue, key domain.RegionKey, window domain.TimeWindow) *domain.Rate {
	if window.Start.IsZero() {
		return nil
	}
	count := 0
	for _, issue := range items {
		if issue.Status != issues.StatusClosed {
			continue
		}
		if issue.ClosedAt == nil {
			continue
		}
		if !BelongsTo(issue, key) {
			continue
		}
		if !inWindow(*issue.ClosedAt, window) {
			continue
		}
		count++
	}
	return rateOver(count, window)
}

// DensityMinMembers is the minimum number of in-region issues with
// embeddings required for ComputeDensity to return a value. Smaller
// samples produce too noisy a centroid to trust.
const DensityMinMembers = 5

// ComputeDensity returns a [0, 1] cohesion score for the region: the
// mean cosine similarity between each in-region issue's embedding and
// the region's centroid (L2-normalized mean of member embeddings).
// Returns nil when fewer than DensityMinMembers issues have embeddings
// or when the centroid is degenerate (zero vector / mismatched dims).
func ComputeDensity(items []issues.Issue, key domain.RegionKey) *float64 {
	members := make([][]float64, 0)
	dim := 0
	for _, issue := range items {
		if !BelongsTo(issue, key) {
			continue
		}
		if len(issue.Embedding) == 0 {
			continue
		}
		if dim == 0 {
			dim = len(issue.Embedding)
		}
		if len(issue.Embedding) != dim {
			continue
		}
		members = append(members, issue.Embedding)
	}
	if len(members) < DensityMinMembers {
		return nil
	}
	centroid := make([]float64, dim)
	for _, vec := range members {
		for i, v := range vec {
			centroid[i] += v
		}
	}
	n := float64(len(members))
	for i := range centroid {
		centroid[i] /= n
	}
	mag := 0.0
	for _, v := range centroid {
		mag += v * v
	}
	if mag == 0 {
		return nil
	}
	sum := 0.0
	for _, vec := range members {
		sum += vectors.CosineSimilarity(vec, centroid)
	}
	density := sum / n
	density = math.Round(density*1000) / 1000
	return &density
}

// OrphanAlignmentFloor is the cosine-similarity threshold below which an
// issue is considered semantically distant from every tag in the catalog.
// The value matches the verifier's verifierFlaggedAlignment constant,
// keeping the corpus-level orphan signal consistent with how the
// verifier classifies individual tag assignments as "flagged."
const OrphanAlignmentFloor = 0.08

// ComputeCorpusOrphans returns the count of issues that don't belong to
// any region and aren't even close to any tag in the catalog. An issue
// is an orphan when BOTH:
//   - its highest TagRelevance is below MembershipFloor (no region
//     membership today), AND
//   - the cosine similarity between its embedding and the nearest tag
//     embedding is below OrphanAlignmentFloor (no semantic neighbor
//     in the catalog).
//
// Issues lacking an embedding are not counted as orphans on the second
// criterion alone (we can't tell); they're skipped.
func ComputeCorpusOrphans(items []issues.Issue, tags []issues.Tag) domain.CorpusOrphans {
	tagEmbeddings := make([][]float64, 0, len(tags))
	for _, t := range tags {
		if len(t.Embedding) > 0 {
			tagEmbeddings = append(tagEmbeddings, t.Embedding)
		}
	}

	var total, open int
	considered := 0
	for _, issue := range items {
		if len(issue.Embedding) == 0 {
			// Can't measure semantic alignment without an embedding; skip.
			continue
		}
		considered++

		topRelevance := 0.0
		for _, score := range issue.TagScores {
			if score.Relevance > topRelevance {
				topRelevance = score.Relevance
			}
		}
		if topRelevance >= MembershipFloor {
			continue
		}

		nearest := 0.0
		for _, tagVec := range tagEmbeddings {
			sim := vectors.CosineSimilarity(issue.Embedding, tagVec)
			if sim > nearest {
				nearest = sim
			}
		}
		if nearest >= OrphanAlignmentFloor {
			continue
		}

		total++
		if issue.Status == issues.StatusOpen {
			open++
		}
	}

	fraction := 0.0
	if considered > 0 {
		fraction = float64(total) / float64(considered)
	}
	return domain.CorpusOrphans{Total: total, Open: open, Fraction: fraction}
}

// ComputeChurn counts events whose IssueID is currently a region member
// and whose timestamp falls in window. Events are pre-filtered to the
// relevant kinds and window by the Store; this function only buckets by
// region. Returns nil for the "all" window (no meaningful denominator).
func ComputeChurn(events []issues.Event, items []issues.Issue, key domain.RegionKey, window domain.TimeWindow) *domain.Rate {
	if window.Start.IsZero() {
		return nil
	}
	inRegion := make(map[string]struct{})
	for _, issue := range items {
		if BelongsTo(issue, key) {
			inRegion[issue.ID] = struct{}{}
		}
	}
	count := 0
	for _, event := range events {
		if _, ok := inRegion[event.IssueID]; ok {
			count++
		}
	}
	return rateOver(count, window)
}

// ComputeMetrics produces all current-phase metrics for a single region.
// Returns false if no issues are members (zero mass).
func ComputeMetrics(items []issues.Issue, events []issues.Event, key domain.RegionKey, window domain.TimeWindow, now time.Time) (domain.RegionMetrics, bool) {
	mass, open, closed := ComputeMass(items, key)
	if mass == 0 {
		return domain.RegionMetrics{}, false
	}
	return domain.RegionMetrics{
		Key:        key,
		Window:     window,
		Mass:       mass,
		MassOpen:   open,
		MassClosed: closed,
		AgeBuckets: ComputeAgeBuckets(items, key, now),
		Density:    ComputeDensity(items, key),
		Growth:     ComputeGrowth(items, key, window),
		Closure:    ComputeClosure(items, key, window),
		Churn:      ComputeChurn(events, items, key, window),
	}, true
}

// inWindow returns true when t is in [window.Start, window.End].
// Endpoints are inclusive so a closure exactly at window.End counts.
func inWindow(t time.Time, window domain.TimeWindow) bool {
	if t.Before(window.Start) {
		return false
	}
	if t.After(window.End) {
		return false
	}
	return true
}

// rateOver normalizes a count by the window's duration in days, rounded
// to two decimal places. Callers must have already short-circuited the
// "all" window before calling.
func rateOver(count int, window domain.TimeWindow) *domain.Rate {
	days := window.End.Sub(window.Start).Hours() / 24
	perDay := 0.0
	if days > 0 {
		perDay = float64(count) / days
	}
	// Round to two decimals so JSON stays compact and stable in tests.
	perDay = math.Round(perDay*100) / 100
	return &domain.Rate{Count: count, PerDay: perDay}
}

// ListRegionsWithMetrics enumerates every tag-region with at least one
// issue at or above MembershipFloor and computes its metrics. Events
// must already be filtered to the relevant kinds and window. Results
// are sorted by Mass descending, ties broken by tag name.
func ListRegionsWithMetrics(items []issues.Issue, events []issues.Event, window domain.TimeWindow, now time.Time) []RegionWithMetrics {
	tags := presentTags(items)
	out := make([]RegionWithMetrics, 0, len(tags))
	for _, tag := range tags {
		key := domain.RegionKey{Kind: domain.RegionKindTag, ID: tag}
		metrics, ok := ComputeMetrics(items, events, key, window, now)
		if !ok {
			continue
		}
		out = append(out, RegionWithMetrics{
			Region:  domain.Region{Key: key, Label: tag},
			Metrics: metrics,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Metrics.Mass != out[j].Metrics.Mass {
			return out[i].Metrics.Mass > out[j].Metrics.Mass
		}
		return out[i].Region.Key.ID < out[j].Region.Key.ID
	})
	return out
}

// presentTags returns the sorted set of normalized tag names that appear
// at relevance >= MembershipFloor on at least one issue.
func presentTags(items []issues.Issue) []string {
	seen := make(map[string]struct{})
	for _, issue := range items {
		for _, score := range issue.TagScores {
			if score.Relevance < MembershipFloor {
				continue
			}
			seen[domain.NormalizeTagName(score.Tag)] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for tag := range seen {
		if tag == "" {
			continue
		}
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}
