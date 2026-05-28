package domain

import "time"

// RegionKind discriminates the source that defines a region's membership.
// Tag-regions derive membership from per-issue tag relevance; cluster and
// custom kinds are reserved for future implementation and rejected at the
// API boundary until then.
type RegionKind string

const (
	RegionKindTag     RegionKind = "tag"
	RegionKindCluster RegionKind = "cluster"
	RegionKindCustom  RegionKind = "custom"
)

// RegionKey is the stable identity of a region. Tag-regions use the
// normalized tag name as ID.
type RegionKey struct {
	Kind RegionKind `json:"kind"`
	ID   string     `json:"id"`
}

// Region is the lightweight descriptor: identity + display. Metrics live
// separately because the same region carries metrics at many windows.
type Region struct {
	Key         RegionKey `json:"key"`
	Label       string    `json:"label"`
	Description string    `json:"description,omitempty"`
}

// TimeWindow is the slice over which growth/closure/churn are measured.
// Mass and age are snapshot quantities — they ignore the window's End/Start
// and read the corpus as of "now."
type TimeWindow struct {
	Label string    `json:"label"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// AgeBucket counts currently-open issues in a region whose age falls into
// the bucket's range.
type AgeBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// Rate is a windowed event count plus its per-day normalization.
type Rate struct {
	Count  int     `json:"count"`
	PerDay float64 `json:"perDay"`
}

// RegionMetrics is the unit returned by the metrics API. Optional fields
// are pointer / slice-nil so phase 1 can ship Mass and AgeBuckets without
// changing the wire shape when later phases populate the rest.
type RegionMetrics struct {
	Key            RegionKey   `json:"key"`
	Window         TimeWindow  `json:"window"`
	Mass           int         `json:"mass"`
	MassOpen       int         `json:"massOpen"`
	MassClosed     int         `json:"massClosed"`
	AgeBuckets     []AgeBucket `json:"ageBuckets,omitempty"`
	Density        *float64    `json:"density,omitempty"`
	Growth         *Rate       `json:"growth,omitempty"`
	Closure        *Rate       `json:"closure,omitempty"`
	Churn          *Rate       `json:"churn,omitempty"`
	AuthorityLinks int         `json:"authorityLinks,omitempty"`
	Authority      *float64    `json:"authority,omitempty"`
}

// CorpusOrphans is the corpus-level "dead zones" signal. Not per-region by
// design: an orphan is precisely an issue that belongs to no region.
type CorpusOrphans struct {
	Total    int     `json:"total"`
	Open     int     `json:"open"`
	Fraction float64 `json:"fraction"`
}

// CustomRegionPredicate is a small intersection-of-conditions predicate.
// Empty/zero fields are ignored; populated fields are AND-combined.
//
//   - Tags: at least one TagPredicate must match (OR semantics within
//     the list).
//   - Statuses: issue.Status must be in this set (when non-empty).
//   - Authors: issue.CreatedBy must be in this set (when non-empty).
//   - MaxAgeDays: (now − CreatedAt).Days must be <= this (when non-nil).
type CustomRegionPredicate struct {
	Tags       []TagPredicate `json:"tags,omitempty"`
	Statuses   []string       `json:"statuses,omitempty"`
	Authors    []string       `json:"authors,omitempty"`
	MaxAgeDays *int           `json:"maxAgeDays,omitempty"`
}

// TagPredicate matches when an issue has the named tag at or above
// MinRelevance.
type TagPredicate struct {
	Tag          string  `json:"tag"`
	MinRelevance float64 `json:"minRelevance"`
}

// CustomRegion is a user-defined region. Stored in the custom_regions
// table; resolved to a membership predicate at compute time.
type CustomRegion struct {
	ID          string                `json:"id"`
	Label       string                `json:"label"`
	Description string                `json:"description,omitempty"`
	Definition  CustomRegionPredicate `json:"definition"`
	CreatedBy   string                `json:"createdBy,omitempty"`
	CreatedAt   time.Time             `json:"createdAt"`
	UpdatedAt   time.Time             `json:"updatedAt"`
}
