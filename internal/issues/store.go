package issues

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"sortit/internal/domain"
)

var ErrNotFound = errors.New("issue not found")
var ErrIssueClosed = errors.New("issue is closed")
var ErrMapProjectionNotFound = errors.New("map projection not found")
var ErrInvalidIssueLink = errors.New("invalid issue link")
var ErrDuplicateIssueLink = errors.New("duplicate issue link")

type IssueStatus string

const (
	StatusOpen   IssueStatus = "open"
	StatusClosed IssueStatus = "closed"
)

type IssueEnrichmentStatus string

const (
	EnrichmentStatusComplete IssueEnrichmentStatus = "complete"
	EnrichmentStatusPending  IssueEnrichmentStatus = "pending"
	EnrichmentStatusFailed   IssueEnrichmentStatus = "failed"
)

type Issue struct {
	ID                       string                 `json:"id"`
	Raw                      string                 `json:"raw"`
	Tags                     []string               `json:"tags"`
	CreatedBy                string                 `json:"createdBy"`
	CreatedAt                time.Time              `json:"createdAt"`
	Status                   IssueStatus            `json:"status"`
	ClosedAt                 *time.Time             `json:"closedAt"`
	ClosedBy                 string                 `json:"closedBy,omitempty"`
	ClosedReason             string                 `json:"closedReason,omitempty"`
	ClosedReasonNote         string                 `json:"closedReasonNote,omitempty"`
	AssignedTo               string                 `json:"assignedTo,omitempty"`
	Discussion               []IssuePost            `json:"discussion,omitempty"`
	Links                    []IssueLink            `json:"links,omitempty"`
	Operations               []IssueOperation       `json:"operations,omitempty"`
	TagScores                []TagRelevance         `json:"tagScores"`
	EnrichmentStatus         IssueEnrichmentStatus  `json:"enrichmentStatus"`
	EnrichmentError          string                 `json:"enrichmentError,omitempty"`
	EnrichmentTargetSequence int                    `json:"enrichmentTargetSequence"`
	LifecycleMetrics         *IssueLifecycleMetrics `json:"lifecycleMetrics,omitempty"`
	Embedding                []float64              `json:"-"`
}

type IssueLifecycleMetrics struct {
	Churn               *float64 `json:"churn,omitempty"`
	Maturity            *float64 `json:"maturity,omitempty"`
	Velocity            *float64 `json:"velocity,omitempty"`
	SnapshotCount       int      `json:"snapshotCount,omitempty"`
	TransitionCount     int      `json:"transitionCount,omitempty"`
	RefinementCount     int      `json:"refinementCount,omitempty"`
	ProgressCount       int      `json:"progressCount,omitempty"`
	RecentActivityCount int      `json:"recentActivityCount,omitempty"`
}

// MapProjectionIssue contains only the fields needed to rebuild the map
// projection and its relationship-driven visibility semantics.
type MapProjectionIssue struct {
	ID         string         `json:"id"`
	Raw        string         `json:"raw"`
	Tags       []string       `json:"tags"`
	Status     IssueStatus    `json:"status"`
	AssignedTo string         `json:"assignedTo,omitempty"`
	TagScores  []TagRelevance `json:"tagScores"`
	Embedding  []float64      `json:"-"`
	Links      []IssueLink    `json:"links,omitempty"`
}

func (i Issue) ReportEvent() Event {
	return Event{
		ID:        IssuePostID(i.ID, 1),
		Kind:      "report",
		IssueID:   i.ID,
		CreatedBy: i.CreatedBy,
		CreatedAt: i.CreatedAt,
		Body:      i.Raw,
	}
}

type IssuePost struct {
	ID        string    `json:"id"`
	IssueID   string    `json:"issueId,omitempty"`
	Raw       string    `json:"raw"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
	Sequence  int       `json:"sequence"`
	Kind      string    `json:"kind,omitempty"`
}

type IssueSnapshot struct {
	IssueID   string         `json:"issueId,omitempty"`
	Sequence  int            `json:"sequence"`
	Raw       string         `json:"raw"`
	Tags      []string       `json:"tags,omitempty"`
	TagScores []TagRelevance `json:"tagScores,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
	Embedding []float64      `json:"-"`
}

type Tag struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	Embedding   []float64 `json:"-"`

	// Specificity scores (nil = not yet computed).
	Specificity           *float64   `json:"specificity,omitempty"`
	SpecificityLLM        *float64   `json:"specificityLlm,omitempty"`
	SpecificityEmbedding  *float64   `json:"specificityEmbedding,omitempty"`
	SpecificityComputedAt *time.Time `json:"specificityComputedAt,omitempty"`
}

// TagRelevance is an alias for domain.TagRelevance.
// All packages should migrate to using domain.TagRelevance directly.
type TagRelevance = domain.TagRelevance

type IssueLinkType string

const (
	IssueLinkTypeParentOf    IssueLinkType = "parent_of"
	IssueLinkTypeChildOf     IssueLinkType = "child_of"
	IssueLinkTypeMergedInto  IssueLinkType = "merged_into"
	IssueLinkTypeDerivedFrom IssueLinkType = "derived_from"
	IssueLinkTypeRelatedTo   IssueLinkType = "related_to"
	IssueLinkTypeDuplicateOf IssueLinkType = "duplicate_of"
)

type IssueOperationKind string

const (
	IssueOperationKindSplit   IssueOperationKind = "split"
	IssueOperationKindCombine IssueOperationKind = "combine"
	IssueOperationKindLink    IssueOperationKind = "link"
)

type IssueReference struct {
	ID     string      `json:"id"`
	Raw    string      `json:"raw"`
	Status IssueStatus `json:"status"`
}

type IssueLink struct {
	ID            string          `json:"id"`
	Type          IssueLinkType   `json:"type"`
	SourceIssueID string          `json:"sourceIssueId"`
	TargetIssueID string          `json:"targetIssueId"`
	Direction     string          `json:"direction,omitempty"`
	CreatedBy     string          `json:"createdBy"`
	CreatedAt     time.Time       `json:"createdAt"`
	Note          string          `json:"note,omitempty"`
	OperationID   string          `json:"operationId,omitempty"`
	RelatedIssue  *IssueReference `json:"relatedIssue,omitempty"`
}

type IssueOperationParticipant struct {
	IssueID string          `json:"issueId"`
	Role    string          `json:"role"`
	Issue   *IssueReference `json:"issue,omitempty"`
}

type IssueOperation struct {
	ID           string                      `json:"id"`
	Kind         IssueOperationKind          `json:"kind"`
	CreatedBy    string                      `json:"createdBy"`
	CreatedAt    time.Time                   `json:"createdAt"`
	Note         string                      `json:"note,omitempty"`
	Participants []IssueOperationParticipant `json:"participants,omitempty"`
}

type SplitChildInput struct {
	Raw       string
	Tags      []string
	TagScores []TagRelevance
	Embedding []float64
}

type SplitInput struct {
	SourceID    string
	Children    []SplitChildInput
	CreatedBy   string
	Note        string
	CloseSource bool
}

type CombineInput struct {
	SourceIDs []string
	Raw       string
	Tags      []string
	CreatedBy string
	Note      string
	TagScores []TagRelevance
	Embedding []float64
}

type LinkInput struct {
	SourceID  string
	TargetID  string
	Type      IssueLinkType
	CreatedBy string
	Note      string
}

type IssueOperationResult struct {
	Operation     IssueOperation   `json:"operation"`
	CreatedIssues []Issue          `json:"createdIssues,omitempty"`
	TouchedIssues []IssueReference `json:"touchedIssues,omitempty"`
}

type CreateInput struct {
	Raw       string
	Tags      []string
	CreatedBy string
	TagScores []TagRelevance
	Embedding []float64
}

type RefineInput struct {
	PostRaw      string
	CanonicalRaw string
	Tags         []string
	CreatedBy    string
	TagScores    []TagRelevance
	Embedding    []float64
}

type ProgressInput struct {
	Raw       string
	CreatedBy string
}

type ListOptions struct {
	Status     IssueStatus
	AssignedTo string
	Tags       []string
	Limit      int
	Offset     int
}

type SemanticSearchOptions struct {
	QueryText      string
	QueryEmbedding []float64
	Status         IssueStatus
	AssignedTo     string
	Tags           []string
	ExcludeID      string
	Limit          int
	Offset         int
	SortBy         string
}

type SemanticSearchResult struct {
	Issue            Issue
	SemanticDistance float64
}

type IssueEmbeddingSimilarity struct {
	ID         string
	Raw        string
	Tags       []string
	Similarity float64
}

type PeopleAnalyticsIssue struct {
	Status     IssueStatus
	AssignedTo string
	TagScores  []TagRelevance
	Embedding  []float64
}

type CompareIssue struct {
	ID        string
	Embedding []float64
}

// EmbeddingActivity is a minimal issue projection used to score how strongly
// recent corpus activity reinforces a memory: the issue's embedding and its most
// recent activity time (creation, or closure when later). It deliberately skips
// the full List hydration (enrichment states, discussion, deep clone) that
// reinforcement scoring never reads.
type EmbeddingActivity struct {
	Embedding  []float64
	ActivityAt time.Time
}

type IssueEnrichmentJob struct {
	IssueID        string
	TargetSequence int
	AttemptCount   int
}

type Reader interface {
	List(context.Context) ([]Issue, error)
	Get(context.Context, string) (Issue, error)
}

type IssueDetailReader interface {
	GetIssueDetail(context.Context, string) (Issue, error)
}

// IssueActivity is the minimal set of issue events needed to compute velocity:
// the issue's posts (refinements/progress) and its links. It deliberately omits
// operations, participants, references, and enrichment so velocity hydration can
// batch-load many issues without paying for full issue detail.
type IssueActivity struct {
	Posts []IssuePost
	Links []IssueLink
}

// IssueActivityReader batch-loads velocity inputs for many issues in a fixed
// number of queries, replacing the per-issue GetIssueDetail fan-out that search
// and explore hydration would otherwise perform. The returned map is keyed by
// issue ID; callers should treat a missing key as "no activity".
type IssueActivityReader interface {
	LoadIssueActivity(context.Context, []string) (map[string]IssueActivity, error)
}

// ReinforcementCandidateReader is the optional slim-projection capability behind
// memory reinforcement scoring: every embedded issue with its most-recent
// activity time, without the full List hydration. PostgresStore and
// InMemoryStore implement it; ObservedStore probes for it and falls back to List
// when a base lacks it.
type ReinforcementCandidateReader interface {
	ListReinforcementCandidates(context.Context) ([]EmbeddingActivity, error)
}

type IssueDetailStore interface {
	GetIssueDetailBase(context.Context, string) (Issue, error)
	ListIssueDetailPosts(context.Context, string) ([]IssuePost, error)
	ListIssueDetailLinks(context.Context, string) ([]IssueLink, error)
	ListIssueDetailOperations(context.Context, string) ([]IssueOperation, error)
	ListIssueDetailReferences(context.Context, []string) ([]IssueReference, error)
	ListIssueSnapshots(context.Context, string) ([]IssueSnapshot, error)
}

type CompareIssueReader interface {
	ListCompareIssues(context.Context, []string) ([]CompareIssue, error)
}

type Writer interface {
	SaveIssue(ctx context.Context, issue Issue) error
	SaveIssuePost(ctx context.Context, post IssuePost) error
	UpdateIssueFields(ctx context.Context, id string, fields IssueFieldUpdate) error
	SaveOperation(ctx context.Context, op IssueOperation) error
	SaveLink(ctx context.Context, link IssueLink) error
}

type Store interface {
	Reader
	Writer
}

type EnrichmentJobWriter interface {
	EnqueueIssueEnrichment(ctx context.Context, issueID string, targetSequence int) error
	CompleteIssueEnrichment(ctx context.Context, issueID string, targetSequence int) error
	RetryIssueEnrichment(ctx context.Context, issueID string, targetSequence int, nextAttemptAt time.Time) error
}

type EnrichmentJobClaimer interface {
	ClaimNextIssueEnrichment(ctx context.Context, leaseDuration time.Duration) (IssueEnrichmentJob, bool, error)
}

// MapProjectionStore optionally provides a bulk read path tailored for
// rebuilding the map projection without hydrating full issue details.
// TagMerger replaces alias tags with the canonical tag across all issues.
type TagMerger interface {
	MergeTags(ctx context.Context, canonical string, aliases []string) error
	DismissTagMerge(ctx context.Context, canonical string, alias string) error
	ListDismissedTagMerges(ctx context.Context) ([]DismissedTagMerge, error)
}

type DismissedTagMerge struct {
	CanonicalName string
	AliasName     string
}

type MapProjectionStore interface {
	LoadMapProjectionData(context.Context) ([]MapProjectionIssue, []Tag, error)
}

type MapProjectionStorePersistence interface {
	GetMapProjection(context.Context, uint64) ([]byte, error)
	SaveMapProjection(context.Context, uint64, []byte) error
}

type MapProjectionInvalidator interface {
	InvalidateMapProjections(context.Context) error
}

type SemanticSearchStore interface {
	SearchIssues(context.Context, SemanticSearchOptions) ([]SemanticSearchResult, error)
}

// IssueFieldUpdate describes which fields to update on an issue.
type IssueFieldUpdate struct {
	Raw                      *string
	Tags                     []string
	TagScores                []TagRelevance
	Embedding                []float64
	LifecycleCreatedAt       *time.Time
	LifecycleCreatedBy       *string
	Status                   *IssueStatus
	ClosedAt                 *time.Time
	ClosedBy                 *string
	ClosedReason             *string
	ClosedReasonNote         *string
	AssignedTo               *string
	EnrichmentStatus         *IssueEnrichmentStatus
	EnrichmentError          *string
	EnrichmentTargetSequence *int
}

var validCloseReasons = map[string]bool{
	"fixed": true, "stale": true, "duplicate": true,
	"wont_fix": true, "by_design": true,
}

func ValidateCloseReason(reason string) error {
	if reason == "" {
		return nil
	}
	if !validCloseReasons[reason] {
		return fmt.Errorf("invalid close reason: %q", reason)
	}
	return nil
}

func DefaultTags() []Tag {
	return []Tag{
		{Name: "bug", Description: "software defect or incorrect behavior. Excludes net-new capabilities or broad product improvements."},
		{Name: "backend", Description: "server-side application logic, APIs, background jobs, auth, or integration behavior implemented off the client. Excludes browser-only runtime issues and persistence-layer work when the database itself is the primary problem."},
		{Name: "crash", Description: "hard failure, freeze, panic, or abrupt termination. Excludes slow or degraded behavior without a hard stop."},
		{Name: "cleanup", Description: "refactoring, dead-code removal, naming cleanup, dependency cleanup, or internal maintenance that improves code health without materially changing user-visible behavior. Excludes bug fixes, new capabilities, and larger architectural redesigns."},
		{Name: "code-organization", Description: "module boundaries, file structure, dependency direction, or splitting and consolidating code for maintainability. Excludes purely mechanical cleanup unless organization or architecture is the primary concern."},
		{Name: "data-persistence", Description: "saving, loading, syncing, retention, or durability of product data across requests, sessions, or restarts. Excludes schema-only work unless it directly affects persisted data behavior."},
		{Name: "database", Description: "database schema, queries, migrations, indexes, constraints, or storage-layer behavior. Excludes general server logic unless the persistence layer itself is the source of the issue."},
		{Name: "feature", Description: "request for a new capability or supported workflow. Excludes fixes and refinements to existing behavior."},
		{Name: "idea", Description: "early concept, exploration, or speculative direction. Excludes concrete scoped feature requests ready for delivery."},
		{Name: "improvement", Description: "refinement to an existing workflow, feature, or system behavior. Excludes net-new capabilities and clear defects."},
		{Name: "ui", Description: "visual presentation, layout, styling, readability, or component rendering. Excludes task-flow friction and non-visual runtime behavior."},
		{Name: "ux", Description: "usability, clarity, discoverability, feedback, or task-flow friction. Excludes purely visual presentation defects and implementation failures."},
		{Name: "frontend", Description: "client-side application logic, browser runtime behavior, or rendering implementation. Excludes pure design concerns unless there is a concrete client behavior problem."},
		{Name: "performance", Description: "latency, throughput, responsiveness, resource usage, or scaling behavior. Excludes hard failures, incorrect output, and purely visual issues."},
		{Name: "safari", Description: "Safari or WebKit-specific compatibility or runtime behavior. Excludes browser-agnostic issues."},
		{Name: "onboarding", Description: "first-run setup, signup, invite, workspace creation, or initial activation flow. Excludes routine use after setup is complete."},
		{Name: "search", Description: "query entry, filtering, ranking, result retrieval, or finding content. Excludes unrelated navigation unless search behavior is central."},
		{Name: "export", Description: "download, file generation, sharing, or data extraction workflows. Excludes general browsing unless data leaves the product."},
	}
}

type InMemoryStore struct {
	mu                sync.RWMutex
	issues            []Issue
	discussion        map[string][]IssuePost
	snapshots         map[string][]IssueSnapshot
	links             []IssueLink
	operations        []IssueOperation
	events            []Event
	enrichmentJobs    map[string]inMemoryEnrichmentJob
	customRegions     map[string]domain.CustomRegion
	memories          map[string]domain.Memory
	memoryProposals   map[string]domain.MemoryProposal
	curationProposals map[string]domain.CurationProposal
}

type inMemoryEnrichmentJob struct {
	IssueEnrichmentJob
	AvailableAt time.Time
}

func NewInMemoryStore(seed []Issue) *InMemoryStore {
	store := &InMemoryStore{
		issues:            cloneIssues(seed),
		discussion:        make(map[string][]IssuePost, len(seed)),
		snapshots:         make(map[string][]IssueSnapshot, len(seed)),
		enrichmentJobs:    make(map[string]inMemoryEnrichmentJob),
		customRegions:     make(map[string]domain.CustomRegion),
		memories:          make(map[string]domain.Memory),
		memoryProposals:   make(map[string]domain.MemoryProposal),
		curationProposals: make(map[string]domain.CurationProposal),
	}

	for _, issue := range store.issues {
		store.discussion[issue.ID] = initialDiscussion(issue)
	}

	slices.SortStableFunc(store.issues, compareIssueOrder)
	return store
}

func (s *InMemoryStore) List(_ context.Context) ([]Issue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := cloneIssues(s.issues)
	for i := range items {
		items[i].Discussion = nil
	}
	return items, nil
}

// ListReinforcementCandidates returns the slim embedding + activity-time
// projection for every embedded issue, mirroring the derivation the full List
// path would feed reinforcement scoring.
func (s *InMemoryStore) ListReinforcementCandidates(_ context.Context) ([]EmbeddingActivity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]EmbeddingActivity, 0, len(s.issues))
	for _, issue := range s.issues {
		if len(issue.Embedding) == 0 {
			continue
		}
		out = append(out, EmbeddingActivity{
			Embedding:  append([]float64(nil), issue.Embedding...),
			ActivityAt: issueActivityAt(issue),
		})
	}
	return out, nil
}

// issueActivityAt is an issue's most-recent activity time: its creation, or its
// closure when that is later.
func issueActivityAt(issue Issue) time.Time {
	at := issue.CreatedAt
	if issue.ClosedAt != nil && issue.ClosedAt.After(at) {
		at = *issue.ClosedAt
	}
	return at
}

func (s *InMemoryStore) Get(_ context.Context, id string) (Issue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id = strings.TrimSpace(id)
	for _, issue := range s.issues {
		if issue.ID == id {
			cloned := cloneIssues([]Issue{issue})[0]
			cloned.Discussion = cloneIssuePosts(s.discussion[id])
			s.hydrateIssueRelationships(&cloned)
			return cloned, nil
		}
	}

	return Issue{}, ErrNotFound
}

func (s *InMemoryStore) GetIssueDetail(ctx context.Context, id string) (Issue, error) {
	return s.Get(ctx, id)
}

func (s *InMemoryStore) GetIssueDetailBase(_ context.Context, id string) (Issue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id = strings.TrimSpace(id)
	for _, issue := range s.issues {
		if issue.ID != id {
			continue
		}
		cloned := cloneIssues([]Issue{issue})[0]
		cloned.Discussion = nil
		cloned.Links = nil
		cloned.Operations = nil
		return cloned, nil
	}

	return Issue{}, ErrNotFound
}

func (s *InMemoryStore) ListIssueDetailPosts(_ context.Context, id string) ([]IssuePost, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id = strings.TrimSpace(id)
	if _, ok := s.discussion[id]; !ok {
		for _, issue := range s.issues {
			if issue.ID == id {
				return nil, nil
			}
		}
		return nil, ErrNotFound
	}

	return cloneIssuePosts(s.discussion[id]), nil
}

func (s *InMemoryStore) ListIssueDetailLinks(_ context.Context, id string) ([]IssueLink, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id = strings.TrimSpace(id)
	found := false
	for _, issue := range s.issues {
		if issue.ID == id {
			found = true
			break
		}
	}
	if !found {
		return nil, ErrNotFound
	}

	links := make([]IssueLink, 0)
	for _, link := range s.links {
		if link.SourceIssueID == id || link.TargetIssueID == id {
			links = append(links, link)
		}
	}
	return cloneIssueLinks(links), nil
}

func (s *InMemoryStore) ListIssueDetailOperations(_ context.Context, id string) ([]IssueOperation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id = strings.TrimSpace(id)
	found := false
	for _, issue := range s.issues {
		if issue.ID == id {
			found = true
			break
		}
	}
	if !found {
		return nil, ErrNotFound
	}

	operations := make([]IssueOperation, 0)
	for _, operation := range s.operations {
		include := false
		for _, participant := range operation.Participants {
			if participant.IssueID == id {
				include = true
				break
			}
		}
		if !include {
			continue
		}
		copyOp := operation
		copyOp.Participants = make([]IssueOperationParticipant, len(operation.Participants))
		for i, participant := range operation.Participants {
			copyOp.Participants[i] = IssueOperationParticipant{
				IssueID: participant.IssueID,
				Role:    participant.Role,
			}
		}
		operations = append(operations, copyOp)
	}
	return cloneIssueOperations(operations), nil
}

func (s *InMemoryStore) ListIssueDetailReferences(_ context.Context, ids []string) ([]IssueReference, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	index := issuesByIDFromList(s.issues)
	refs := make([]IssueReference, 0, len(ids))
	for _, id := range SanitizeIssueIDs(ids) {
		issue, ok := index[id]
		if !ok {
			continue
		}
		refs = append(refs, issueReference(issue))
	}
	return refs, nil
}

func (s *InMemoryStore) LoadMapProjectionData(_ context.Context) ([]MapProjectionIssue, []Tag, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := cloneIssues(s.issues)
	for i := range items {
		items[i].Discussion = nil
		s.hydrateIssueRelationships(&items[i])
	}

	return MapProjectionIssuesFromIssues(items), nil, nil
}

func (s *InMemoryStore) Replace(_ context.Context, next []Issue) error {
	items := cloneIssues(next)
	slices.SortStableFunc(items, compareIssueOrder)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.issues = items
	s.discussion = make(map[string][]IssuePost, len(items))
	s.links = nil
	s.operations = nil
	s.events = nil
	s.enrichmentJobs = make(map[string]inMemoryEnrichmentJob)
	for _, issue := range items {
		s.discussion[issue.ID] = initialDiscussion(issue)
	}
	return nil
}

func sanitizeTags(tags []string) []string {
	if len(tags) == 0 {
		return []string{}
	}

	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tag)
	}

	if len(out) == 0 {
		return []string{}
	}
	return out
}

func displayTags(explicitTags []string, scores []TagRelevance) []string {
	return displayTagsWithSpecificity(explicitTags, scores, nil)
}

func displayTagsWithSpecificity(explicitTags []string, scores []TagRelevance, tagSpecificity map[string]*float64) []string {
	if tags := sanitizeTags(explicitTags); len(tags) > 0 {
		return tags
	}
	if len(scores) == 0 {
		return []string{}
	}

	normalized := copyTagScores(scores)
	slices.SortStableFunc(normalized, func(a, b TagRelevance) int {
		aScore := displayScore(a.Relevance, tagSpecificity, a.Tag)
		bScore := displayScore(b.Relevance, tagSpecificity, b.Tag)
		if aScore > bScore {
			return -1
		}
		if aScore < bScore {
			return 1
		}
		if a.Tag < b.Tag {
			return -1
		}
		if a.Tag > b.Tag {
			return 1
		}
		return 0
	})

	out := make([]string, 0, min(3, len(normalized)))
	for _, score := range normalized {
		if score.Tag == "" {
			continue
		}
		if len(out) > 0 && score.Relevance < 0.2 {
			break
		}
		out = append(out, score.Tag)
		if len(out) == 3 {
			break
		}
	}
	if len(out) == 0 && normalized[0].Tag != "" {
		return []string{normalized[0].Tag}
	}
	return out
}

// displayScore blends relevance and specificity for display tag ranking.
// displayScore = relevance * 0.5 + specificity * 0.5
func displayScore(relevance float64, tagSpecificity map[string]*float64, tagName string) float64 {
	if tagSpecificity == nil {
		return relevance
	}
	s := 0.5
	if p, ok := tagSpecificity[tagName]; ok && p != nil {
		s = *p
	}
	return relevance*0.5 + s*0.5
}

func normalizeIssueEnrichment(issue Issue) Issue {
	issue.EnrichmentStatus = normalizeIssueEnrichmentStatus(issue.EnrichmentStatus)
	if issue.EnrichmentStatus == "" {
		issue.EnrichmentStatus = EnrichmentStatusComplete
	}
	if issue.EnrichmentTargetSequence <= 0 {
		issue.EnrichmentTargetSequence = max(1, len(issue.Discussion))
	}
	if issue.EnrichmentStatus == EnrichmentStatusComplete {
		issue.EnrichmentError = ""
	}
	return issue
}

func normalizeIssueEnrichmentStatus(status IssueEnrichmentStatus) IssueEnrichmentStatus {
	switch status {
	case EnrichmentStatusPending, EnrichmentStatusFailed:
		return status
	default:
		return EnrichmentStatusComplete
	}
}

func cloneIssues(input []Issue) []Issue {
	items := make([]Issue, len(input))
	for i, issue := range input {
		issue = normalizeIssueEnrichment(issue)
		items[i] = Issue{
			ID:                       issue.ID,
			Raw:                      issue.Raw,
			Tags:                     append([]string(nil), issue.Tags...),
			CreatedBy:                issue.CreatedBy,
			CreatedAt:                issue.CreatedAt,
			Status:                   normalizeIssueStatus(issue.Status),
			ClosedAt:                 cloneTimePtr(issue.ClosedAt),
			ClosedBy:                 issue.ClosedBy,
			ClosedReason:             issue.ClosedReason,
			ClosedReasonNote:         issue.ClosedReasonNote,
			AssignedTo:               issue.AssignedTo,
			Discussion:               cloneIssuePosts(issue.Discussion),
			Links:                    cloneIssueLinks(issue.Links),
			Operations:               cloneIssueOperations(issue.Operations),
			TagScores:                copyTagScores(issue.TagScores),
			EnrichmentStatus:         issue.EnrichmentStatus,
			EnrichmentError:          issue.EnrichmentError,
			EnrichmentTargetSequence: issue.EnrichmentTargetSequence,
			LifecycleMetrics:         cloneIssueLifecycleMetrics(issue.LifecycleMetrics),
			Embedding:                copyEmbedding(issue.Embedding),
		}
	}
	return items
}

func cloneIssueLifecycleMetrics(value *IssueLifecycleMetrics) *IssueLifecycleMetrics {
	if value == nil {
		return nil
	}

	return &IssueLifecycleMetrics{
		Churn:               cloneFloat64Ptr(value.Churn),
		Maturity:            cloneFloat64Ptr(value.Maturity),
		Velocity:            cloneFloat64Ptr(value.Velocity),
		SnapshotCount:       value.SnapshotCount,
		TransitionCount:     value.TransitionCount,
		RefinementCount:     value.RefinementCount,
		ProgressCount:       value.ProgressCount,
		RecentActivityCount: value.RecentActivityCount,
	}
}

func cloneIssueSnapshots(input []IssueSnapshot) []IssueSnapshot {
	if len(input) == 0 {
		return nil
	}

	items := make([]IssueSnapshot, len(input))
	for i, snapshot := range input {
		items[i] = IssueSnapshot{
			IssueID:   snapshot.IssueID,
			Sequence:  snapshot.Sequence,
			Raw:       snapshot.Raw,
			Tags:      append([]string(nil), snapshot.Tags...),
			TagScores: copyTagScores(snapshot.TagScores),
			CreatedAt: snapshot.CreatedAt,
			Embedding: copyEmbedding(snapshot.Embedding),
		}
	}
	return items
}

func cloneMapProjectionIssues(input []MapProjectionIssue) []MapProjectionIssue {
	items := make([]MapProjectionIssue, len(input))
	for i, issue := range input {
		items[i] = MapProjectionIssue{
			ID:         issue.ID,
			Raw:        issue.Raw,
			Tags:       append([]string(nil), issue.Tags...),
			Status:     normalizeIssueStatus(issue.Status),
			AssignedTo: issue.AssignedTo,
			TagScores:  copyTagScores(issue.TagScores),
			Embedding:  copyEmbedding(issue.Embedding),
			Links:      cloneIssueLinks(issue.Links),
		}
	}
	return items
}

func cloneIssuePosts(input []IssuePost) []IssuePost {
	if len(input) == 0 {
		return nil
	}

	items := make([]IssuePost, len(input))
	for i, post := range input {
		items[i] = IssuePost{
			ID:        post.ID,
			IssueID:   post.IssueID,
			Raw:       post.Raw,
			CreatedBy: post.CreatedBy,
			CreatedAt: post.CreatedAt,
			Sequence:  post.Sequence,
			Kind:      post.Kind,
		}
	}
	return items
}

func cloneIssueLinks(input []IssueLink) []IssueLink {
	if len(input) == 0 {
		return nil
	}

	items := make([]IssueLink, len(input))
	for i, link := range input {
		items[i] = IssueLink{
			ID:            link.ID,
			Type:          link.Type,
			SourceIssueID: link.SourceIssueID,
			TargetIssueID: link.TargetIssueID,
			Direction:     link.Direction,
			CreatedBy:     link.CreatedBy,
			CreatedAt:     link.CreatedAt,
			Note:          link.Note,
			OperationID:   link.OperationID,
			RelatedIssue:  cloneIssueReference(link.RelatedIssue),
		}
	}
	return items
}

func issueIDs(items []Issue) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func cloneIssueOperations(input []IssueOperation) []IssueOperation {
	if len(input) == 0 {
		return nil
	}

	items := make([]IssueOperation, len(input))
	for i, operation := range input {
		participants := make([]IssueOperationParticipant, len(operation.Participants))
		for j, participant := range operation.Participants {
			participants[j] = IssueOperationParticipant{
				IssueID: participant.IssueID,
				Role:    participant.Role,
				Issue:   cloneIssueReference(participant.Issue),
			}
		}
		items[i] = IssueOperation{
			ID:           operation.ID,
			Kind:         operation.Kind,
			CreatedBy:    operation.CreatedBy,
			CreatedAt:    operation.CreatedAt,
			Note:         operation.Note,
			Participants: participants,
		}
	}
	return items
}

func cloneIssueReference(input *IssueReference) *IssueReference {
	if input == nil {
		return nil
	}
	cloned := *input
	return &cloned
}

func cloneTags(input []Tag) []Tag {
	items := make([]Tag, len(input))
	for i, tag := range input {
		items[i] = Tag{
			Name:                  tag.Name,
			Description:           tag.Description,
			CreatedAt:             tag.CreatedAt,
			Embedding:             copyEmbedding(tag.Embedding),
			Specificity:           copyFloat64Ptr(tag.Specificity),
			SpecificityLLM:        copyFloat64Ptr(tag.SpecificityLLM),
			SpecificityEmbedding:  copyFloat64Ptr(tag.SpecificityEmbedding),
			SpecificityComputedAt: copyTimePtr(tag.SpecificityComputedAt),
		}
	}
	return items
}

func copyFloat64Ptr(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func copyTimePtr(p *time.Time) *time.Time {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func sanitizeTagName(name string) string {
	return domain.NormalizeTagName(name)
}

func copyTagScores(input []TagRelevance) []TagRelevance {
	if len(input) == 0 {
		return nil
	}
	out := make([]TagRelevance, len(input))
	for i, score := range input {
		out[i] = score
		out[i].CandidateSources = append([]string(nil), score.CandidateSources...)
		out[i].Alignment = copyFloat64Ptr(score.Alignment)
		out[i].Specificity = copyFloat64Ptr(score.Specificity)
		out[i].DominanceGap = copyFloat64Ptr(score.DominanceGap)
		out[i].Evidence = append([]domain.EvidenceRange(nil), score.Evidence...)
		out[i].Negation = copyFloat64Ptr(score.Negation)
		out[i].NegationEvidence = append([]domain.EvidenceRange(nil), score.NegationEvidence...)
	}
	return out
}

func copyEmbedding(input []float64) []float64 {
	if len(input) == 0 {
		return nil
	}
	out := make([]float64, len(input))
	copy(out, input)
	return out
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	cloned := value.UTC()
	return &cloned
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}

	cloned := *value
	return &cloned
}

func normalizeIssueStatus(status IssueStatus) IssueStatus {
	if strings.EqualFold(strings.TrimSpace(string(status)), string(StatusClosed)) {
		return StatusClosed
	}
	return StatusOpen
}

func defaultActor(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "You"
	}
	return value
}

func initialDiscussion(issue Issue) []IssuePost {
	if len(issue.Discussion) > 0 {
		return cloneIssuePosts(issue.Discussion)
	}

	return []IssuePost{{
		ID:        issuePostID(issue.ID, 1),
		IssueID:   issue.ID,
		Raw:       issue.Raw,
		CreatedBy: issue.CreatedBy,
		CreatedAt: issue.CreatedAt,
		Sequence:  1,
	}}
}

func issuePostID(issueID string, sequence int) string {
	return fmt.Sprintf("%s-post-%06d", strings.TrimSpace(issueID), sequence)
}

func compareIssueOrder(a, b Issue) int {
	if result := b.CreatedAt.Compare(a.CreatedAt); result != 0 {
		return result
	}
	if a.ID < b.ID {
		return -1
	}
	if a.ID > b.ID {
		return 1
	}
	return 0
}

func normalizeIssueLinkType(value IssueLinkType) IssueLinkType {
	switch strings.TrimSpace(string(value)) {
	case string(IssueLinkTypeParentOf):
		return IssueLinkTypeParentOf
	case string(IssueLinkTypeChildOf):
		return IssueLinkTypeChildOf
	case string(IssueLinkTypeMergedInto):
		return IssueLinkTypeMergedInto
	case string(IssueLinkTypeDerivedFrom):
		return IssueLinkTypeDerivedFrom
	case string(IssueLinkTypeRelatedTo):
		return IssueLinkTypeRelatedTo
	case string(IssueLinkTypeDuplicateOf):
		return IssueLinkTypeDuplicateOf
	default:
		return ""
	}
}

func sanitizeIssueIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func issueReference(issue Issue) IssueReference {
	return IssueReference{
		ID:     issue.ID,
		Raw:    issue.Raw,
		Status: normalizeIssueStatus(issue.Status),
	}
}

func issuesByIDFromList(items []Issue) map[string]Issue {
	index := make(map[string]Issue, len(items))
	for _, issue := range items {
		index[issue.ID] = issue
	}
	return index
}

func hydrateOperation(operation IssueOperation, issuesByID map[string]Issue) IssueOperation {
	hydrated := operation
	hydrated.Participants = make([]IssueOperationParticipant, len(operation.Participants))
	for i, participant := range operation.Participants {
		hydrated.Participants[i] = participant
		if issue, ok := issuesByID[participant.IssueID]; ok {
			ref := issueReference(issue)
			hydrated.Participants[i].Issue = &ref
		}
	}
	return hydrated
}

func (s *InMemoryStore) hydrateIssueRelationships(issue *Issue) {
	issuesByID := issuesByIDFromList(s.issues)
	links := make([]IssueLink, 0)
	for _, link := range s.links {
		if link.SourceIssueID != issue.ID && link.TargetIssueID != issue.ID {
			continue
		}
		hydrated := link
		if hydrated.SourceIssueID == issue.ID {
			hydrated.Direction = "outgoing"
			if related, ok := issuesByID[hydrated.TargetIssueID]; ok {
				ref := issueReference(related)
				hydrated.RelatedIssue = &ref
			}
		} else {
			hydrated.Direction = "incoming"
			if related, ok := issuesByID[hydrated.SourceIssueID]; ok {
				ref := issueReference(related)
				hydrated.RelatedIssue = &ref
			}
		}
		links = append(links, hydrated)
	}
	operations := make([]IssueOperation, 0)
	for _, operation := range s.operations {
		for _, participant := range operation.Participants {
			if participant.IssueID == issue.ID {
				operations = append(operations, hydrateOperation(operation, issuesByID))
				break
			}
		}
	}
	issue.Links = cloneIssueLinks(links)
	issue.Operations = cloneIssueOperations(operations)
}
