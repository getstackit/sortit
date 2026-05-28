// Package scoring defines shared constants for Sortit's scoring and ranking
// model. All tunable weights, thresholds, and limits used across search,
// explore, person recommendations, and graph signals live here so they can
// be reviewed and adjusted in one place.
package scoring

// Signal blend weights: how semantic and factor similarity are combined.
const (
	SemanticWeight            = 0.6  // default semantic share of the blend
	FactorWeight              = 0.4  // default factor share of the blend
	TagCorrelationSemantic    = 0.5  // semantic share when query matches a tag name
	TagCorrelationFactor      = 0.5  // factor share when query matches a tag name
	PersonRecommendFactor     = 0.65 // factor share for person recommendations
	PersonRecommendSemantic   = 0.35 // semantic share for person recommendations
	CorrelationSemanticWeight = 0.6  // semantic share for work correlations
	CorrelationFactorWeight   = 0.4  // factor share for work correlations
)

// Authority and hubness: graph-derived scoring signals.
const (
	AuthorityPerLink    = 0.25 // authority gained per inbound duplicate_of/merged_into link
	AuthorityConsumerWt = 0.1  // additive authority boost in search, explore, person recs
	HubnessPerLink      = 0.15 // hubness gained per link (any type)
)

// Time decay: freshness weighting.
const (
	FreshnessFloor        = 0.3  // minimum weight regardless of age
	FreshnessHalfLifeDays = 90.0 // days until weight reaches midpoint
)

// Search ranking modifiers.
const (
	SearchVelocityBoost        = 0.08 // multiplicative velocity contribution
	ContentConfidenceTieWindow = 0.05 // score gap below which content confidence breaks ties
	SpecificityPenaltyPerTag   = 0.04 // penalty per generic tag in top-N
	SpecificityPenaltyTopN     = 3    // how many top tags to check for specificity
	CooccurrenceBoostPerTag    = 0.03 // boost per specific co-occurring tag
	CooccurrenceBoostMax       = 0.06 // cap on co-occurrence boost
	GenericTagThreshold        = 0.5  // specificity below this is considered generic
	CooccurrenceRelevanceMin   = 0.2  // minimum tag relevance to consider for co-occurrence
)

// Person recommendation scoring.
const (
	PersonMaturityBase    = 0.8  // base maturity factor
	PersonMaturityWeight  = 0.2  // maturity multiplier weight
	PersonVelocityPenalty = 0.15 // velocity penalty factor
	PersonMaxRecommend    = 5    // maximum recommendations returned
	PersonLowSignal       = 0.15 // filter threshold for factor/semantic scores
	PersonReasonFactor    = 0.45 // factor score threshold for reason text
	PersonReasonSemantic  = 0.45 // semantic score threshold for reason text
)

// Explore scoring.
const (
	ExploreOpportunityMinSim = 0.45 // minimum combined similarity for opportunities
	ExploreMaturityBase      = 0.8  // base maturity factor in opportunity confidence
	ExploreMaturityWeight    = 0.2  // maturity multiplier weight
	ExploreVelocityBase      = 0.9  // base velocity factor in opportunity confidence
	ExploreVelocityWeight    = 0.1  // velocity multiplier weight
)

// Explore/search reason thresholds.
const (
	ReasonFactorSemanticGap = 0.05 // factor >= semantic - this triggers tag-based reason
	ReasonHighSemantic      = 0.75 // semantic above this triggers "semantically similar" reason
	ReasonHighFactor        = 0.55 // factor above this triggers "similar factor profile" reason
)

// Shared display limits.
const (
	DefaultResultLimit = 8 // default search/explore result count
	SharedTagsLimit    = 3 // max shared tags shown in results
	OpportunityTagsMax = 2 // max shared tags in opportunity grouping
)

// Factor decomposition.
const (
	MinFactorWeight           = 0.05 // floor: never fully ignore factors
	MaxFactorWeight           = 0.95 // ceiling: never fully ignore residuals
	TagCorrelationFactorNudge = 0.1  // additional factor weight when query matches a tag
	MinDecompositionIssues    = 5    // below this, fall back to hardcoded weights
)

// Region-aware search. Applied when a search query identifies a target
// tag-region (e.g., the query string matches a known tag name).
const (
	RegionMembershipFloor          = 0.4  // mirrors regions.MembershipFloor
	RegionMatchBoost               = 0.08 // additive boost for in-region candidates
	RegionAntiCorrelationPenalty   = 0.12 // multiplier for the implicit-negative weight
	RegionAntiCorrelationStrongTag = 0.4  // ignore anti-correlators on weakly-tagged candidates
)

// Projection weighting.
const (
	ProjectionWeightFloor = 0.1 // minimum per-issue weight in PCA covariance
	DefaultMaturity       = 0.5 // maturity fallback when lifecycle metrics are absent
)
