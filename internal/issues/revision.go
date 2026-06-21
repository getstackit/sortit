package issues

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"sortit/internal/domain"
)

// RevisionTracker is the in-process implementation of RevisionBus. It holds an
// atomic revision counter and a set of subscriber channels that receive
// notifications after each Bump.
type RevisionTracker struct {
	revision atomic.Uint64

	mu          sync.Mutex
	nextSubID   uint64
	subscribers map[uint64]chan uint64
}

func NewRevisionTracker() *RevisionTracker {
	tracker := &RevisionTracker{
		subscribers: make(map[uint64]chan uint64),
	}
	tracker.revision.Store(1)
	return tracker
}

func (t *RevisionTracker) Revision() uint64 {
	if t == nil {
		return 0
	}
	return t.revision.Load()
}

func (t *RevisionTracker) Bump() uint64 {
	if t == nil {
		return 0
	}
	next := t.revision.Add(1)
	t.fanout(next)
	return next
}

// Subscribe registers a subscriber. Delivery is drop-latest on a cap-1 channel:
// slow subscribers miss intermediate revisions but always converge to the
// newest value, which matches the semantics the UI previously got from
// polling. Callers MUST invoke cancel to release resources.
func (t *RevisionTracker) Subscribe() (<-chan uint64, func()) {
	if t == nil {
		ch := make(chan uint64)
		close(ch)
		return ch, func() {}
	}

	ch := make(chan uint64, 1)
	t.mu.Lock()
	id := t.nextSubID
	t.nextSubID++
	if t.subscribers == nil {
		t.subscribers = make(map[uint64]chan uint64)
	}
	t.subscribers[id] = ch
	t.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			t.mu.Lock()
			if sub, ok := t.subscribers[id]; ok {
				delete(t.subscribers, id)
				close(sub)
			}
			t.mu.Unlock()
		})
	}
	return ch, cancel
}

func (t *RevisionTracker) fanout(revision uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, ch := range t.subscribers {
		// Drop-latest: replace any pending value with the newest.
		select {
		case ch <- revision:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- revision:
			default:
			}
		}
	}
}

type ObservedStore struct {
	base       Store
	eventStore EventStore
	tracker    *RevisionTracker
}

func NewObservedStore(base Store, tracker *RevisionTracker) *ObservedStore {
	if base == nil {
		return nil
	}
	es, _ := base.(EventStore)
	return &ObservedStore{base: base, eventStore: es, tracker: tracker}
}

func (s *ObservedStore) RecordEvent(ctx context.Context, event Event) error {
	if s.eventStore == nil {
		return nil
	}
	return s.eventStore.RecordEvent(ctx, event)
}

func (s *ObservedStore) ListEvents(ctx context.Context, limit int, cursor string, kind string) ([]Event, string, error) {
	if s.eventStore == nil {
		return nil, "", nil
	}
	return s.eventStore.ListEvents(ctx, limit, cursor, kind)
}

func (s *ObservedStore) ListLifecycleEvents(ctx context.Context, kinds []string, start, end time.Time) ([]Event, error) {
	if s.eventStore == nil {
		return nil, nil
	}
	return s.eventStore.ListLifecycleEvents(ctx, kinds, start, end)
}

func (s *ObservedStore) ListCustomRegions(ctx context.Context) ([]domain.CustomRegion, error) {
	if cr, ok := s.base.(CustomRegionStore); ok {
		return cr.ListCustomRegions(ctx)
	}
	return nil, nil
}

func (s *ObservedStore) GetCustomRegion(ctx context.Context, id string) (domain.CustomRegion, error) {
	if cr, ok := s.base.(CustomRegionStore); ok {
		return cr.GetCustomRegion(ctx, id)
	}
	return domain.CustomRegion{}, ErrCustomRegionNotFound
}

func (s *ObservedStore) UpsertCustomRegion(ctx context.Context, region domain.CustomRegion) error {
	if cr, ok := s.base.(CustomRegionStore); ok {
		if err := cr.UpsertCustomRegion(ctx, region); err != nil {
			return err
		}
		s.tracker.Bump()
		return nil
	}
	return ErrCustomRegionNotFound
}

func (s *ObservedStore) DeleteCustomRegion(ctx context.Context, id string) error {
	if cr, ok := s.base.(CustomRegionStore); ok {
		if err := cr.DeleteCustomRegion(ctx, id); err != nil {
			return err
		}
		s.tracker.Bump()
		return nil
	}
	return nil
}

func (s *ObservedStore) ListMemories(ctx context.Context, opts MemoryListOptions) ([]domain.Memory, error) {
	if ms, ok := s.base.(MemoryStore); ok {
		return ms.ListMemories(ctx, opts)
	}
	return nil, nil
}

func (s *ObservedStore) GetMemory(ctx context.Context, id string) (domain.Memory, error) {
	if ms, ok := s.base.(MemoryStore); ok {
		return ms.GetMemory(ctx, id)
	}
	return domain.Memory{}, ErrMemoryNotFound
}

func (s *ObservedStore) GetActiveConceptBySubjectTag(ctx context.Context, subjectTag string) (domain.Memory, error) {
	if ms, ok := s.base.(MemoryStore); ok {
		return ms.GetActiveConceptBySubjectTag(ctx, subjectTag)
	}
	return domain.Memory{}, ErrMemoryNotFound
}

func (s *ObservedStore) GetActiveOverview(ctx context.Context) (domain.Memory, error) {
	if ms, ok := s.base.(MemoryStore); ok {
		return ms.GetActiveOverview(ctx)
	}
	return domain.Memory{}, ErrMemoryNotFound
}

func (s *ObservedStore) UpsertMemory(ctx context.Context, memory domain.Memory) error {
	if ms, ok := s.base.(MemoryStore); ok {
		if err := ms.UpsertMemory(ctx, memory); err != nil {
			return err
		}
		s.tracker.Bump()
		return nil
	}
	return ErrMemoryNotFound
}

func (s *ObservedStore) DeleteMemory(ctx context.Context, id string) error {
	if ms, ok := s.base.(MemoryStore); ok {
		if err := ms.DeleteMemory(ctx, id); err != nil {
			return err
		}
		s.tracker.Bump()
		return nil
	}
	return nil
}

func (s *ObservedStore) SearchMemories(ctx context.Context, query []float64, limit int) ([]MemorySimilarity, error) {
	if ms, ok := s.base.(MemoryStore); ok {
		return ms.SearchMemories(ctx, query, limit)
	}
	return nil, nil
}

func (s *ObservedStore) ListMemoryProposals(ctx context.Context, status domain.MemoryProposalStatus) ([]domain.MemoryProposal, error) {
	if ps, ok := s.base.(MemoryProposalStore); ok {
		return ps.ListMemoryProposals(ctx, status)
	}
	return nil, nil
}

func (s *ObservedStore) GetMemoryProposal(ctx context.Context, id string) (domain.MemoryProposal, error) {
	if ps, ok := s.base.(MemoryProposalStore); ok {
		return ps.GetMemoryProposal(ctx, id)
	}
	return domain.MemoryProposal{}, ErrMemoryProposalNotFound
}

func (s *ObservedStore) UpsertMemoryProposal(ctx context.Context, proposal domain.MemoryProposal) error {
	if ps, ok := s.base.(MemoryProposalStore); ok {
		if err := ps.UpsertMemoryProposal(ctx, proposal); err != nil {
			return err
		}
		s.tracker.Bump()
		return nil
	}
	return ErrMemoryProposalNotFound
}

func (s *ObservedStore) ListCurationProposals(ctx context.Context, status domain.CurationProposalStatus) ([]domain.CurationProposal, error) {
	if ps, ok := s.base.(CurationProposalStore); ok {
		return ps.ListCurationProposals(ctx, status)
	}
	return nil, nil
}

func (s *ObservedStore) GetCurationProposal(ctx context.Context, id string) (domain.CurationProposal, error) {
	if ps, ok := s.base.(CurationProposalStore); ok {
		return ps.GetCurationProposal(ctx, id)
	}
	return domain.CurationProposal{}, ErrCurationProposalNotFound
}

func (s *ObservedStore) UpsertCurationProposal(ctx context.Context, proposal domain.CurationProposal) error {
	if ps, ok := s.base.(CurationProposalStore); ok {
		if err := ps.UpsertCurationProposal(ctx, proposal); err != nil {
			return err
		}
		s.tracker.Bump()
		return nil
	}
	return ErrCurationProposalNotFound
}

func (s *ObservedStore) List(ctx context.Context) ([]Issue, error) {
	return s.base.List(ctx)
}

func (s *ObservedStore) Get(ctx context.Context, id string) (Issue, error) {
	return s.base.Get(ctx, id)
}

func (s *ObservedStore) GetIssueDetail(ctx context.Context, id string) (Issue, error) {
	if detailReader, ok := s.base.(IssueDetailReader); ok {
		return detailReader.GetIssueDetail(ctx, id)
	}
	return s.base.Get(ctx, id)
}

func (s *ObservedStore) LoadIssueActivity(ctx context.Context, ids []string) (map[string]IssueActivity, error) {
	if reader, ok := s.base.(IssueActivityReader); ok {
		return reader.LoadIssueActivity(ctx, ids)
	}
	// Fallback for bases without batch support (e.g. the in-memory store): derive
	// activity from per-issue detail. This mirrors the pre-batch hydration, so a
	// non-batching base behaves exactly as before.
	activity := make(map[string]IssueActivity, len(ids))
	for _, id := range ids {
		detail, err := s.GetIssueDetail(ctx, id)
		if err != nil {
			continue
		}
		activity[id] = IssueActivity{Posts: detail.Discussion, Links: detail.Links}
	}
	return activity, nil
}

func (s *ObservedStore) ListReinforcementCandidates(ctx context.Context) ([]EmbeddingActivity, error) {
	if reader, ok := s.base.(ReinforcementCandidateReader); ok {
		return reader.ListReinforcementCandidates(ctx)
	}
	// Fallback for bases without the slim projection: derive from the full List.
	// Mirrors the pre-optimization behavior exactly.
	all, err := s.base.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]EmbeddingActivity, 0, len(all))
	for _, issue := range all {
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

func (s *ObservedStore) GetIssueDetailBase(ctx context.Context, id string) (Issue, error) {
	if detailStore, ok := s.base.(IssueDetailStore); ok {
		return detailStore.GetIssueDetailBase(ctx, id)
	}
	issue, err := s.base.Get(ctx, id)
	if err != nil {
		return Issue{}, err
	}
	issue.Discussion = nil
	issue.Links = nil
	issue.Operations = nil
	return issue, nil
}

func (s *ObservedStore) ListIssueDetailPosts(ctx context.Context, id string) ([]IssuePost, error) {
	if detailStore, ok := s.base.(IssueDetailStore); ok {
		return detailStore.ListIssueDetailPosts(ctx, id)
	}
	issue, err := s.base.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return cloneIssuePosts(issue.Discussion), nil
}

func (s *ObservedStore) ListIssueDetailLinks(ctx context.Context, id string) ([]IssueLink, error) {
	if detailStore, ok := s.base.(IssueDetailStore); ok {
		return detailStore.ListIssueDetailLinks(ctx, id)
	}
	issue, err := s.base.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return cloneIssueLinks(issue.Links), nil
}

func (s *ObservedStore) ListIssueDetailOperations(ctx context.Context, id string) ([]IssueOperation, error) {
	if detailStore, ok := s.base.(IssueDetailStore); ok {
		return detailStore.ListIssueDetailOperations(ctx, id)
	}
	issue, err := s.base.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return cloneIssueOperations(issue.Operations), nil
}

func (s *ObservedStore) ListIssueDetailReferences(ctx context.Context, ids []string) ([]IssueReference, error) {
	if detailStore, ok := s.base.(IssueDetailStore); ok {
		return detailStore.ListIssueDetailReferences(ctx, ids)
	}

	refs := make([]IssueReference, 0, len(ids))
	for _, id := range SanitizeIssueIDs(ids) {
		issue, err := s.base.Get(ctx, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		refs = append(refs, issueReference(issue))
	}
	return refs, nil
}

func (s *ObservedStore) ListIssueSnapshots(ctx context.Context, id string) ([]IssueSnapshot, error) {
	if detailStore, ok := s.base.(IssueDetailStore); ok {
		return detailStore.ListIssueSnapshots(ctx, id)
	}
	return nil, nil
}

func (s *ObservedStore) SaveIssue(ctx context.Context, issue Issue) error {
	if err := s.base.SaveIssue(ctx, issue); err != nil {
		return err
	}
	s.tracker.Bump()
	return nil
}

func (s *ObservedStore) SaveIssuePost(ctx context.Context, post IssuePost) error {
	if err := s.base.SaveIssuePost(ctx, post); err != nil {
		return err
	}
	s.tracker.Bump()
	return nil
}

func (s *ObservedStore) UpdateIssueFields(ctx context.Context, id string, fields IssueFieldUpdate) error {
	if err := s.base.UpdateIssueFields(ctx, id, fields); err != nil {
		return err
	}
	s.tracker.Bump()
	return nil
}

func (s *ObservedStore) SaveOperation(ctx context.Context, op IssueOperation) error {
	if err := s.base.SaveOperation(ctx, op); err != nil {
		return err
	}
	s.tracker.Bump()
	return nil
}

func (s *ObservedStore) SaveLink(ctx context.Context, link IssueLink) error {
	if err := s.base.SaveLink(ctx, link); err != nil {
		return err
	}
	s.tracker.Bump()
	return nil
}

type observedTagStore interface {
	ListTags(context.Context) ([]Tag, error)
	UpsertTags(context.Context, []Tag) error
	UpdateTagSpecificity(ctx context.Context, name string, specificity, llm, embedding *float64, computedAt *time.Time) error
}

type observedMapProjectionStore interface {
	LoadMapProjectionData(context.Context) ([]MapProjectionIssue, []Tag, error)
}

type observedIssueMetadataStore interface {
	ListIssueMetadata(context.Context) ([]Issue, error)
}

type observedIssueEmbeddingSimilarityStore interface {
	ListIssueEmbeddingSimilarities(context.Context, []float64, int) ([]IssueEmbeddingSimilarity, int, float64, error)
}

type observedPeopleAnalyticsStore interface {
	ListPeopleAnalytics(context.Context, ListOptions) ([]PeopleAnalyticsIssue, error)
}

type observedCompareIssueStore interface {
	ListCompareIssues(context.Context, []string) ([]CompareIssue, error)
}

func (s *ObservedStore) ListTags(ctx context.Context) ([]Tag, error) {
	tagStore, ok := s.base.(observedTagStore)
	if !ok {
		return nil, nil
	}
	return tagStore.ListTags(ctx)
}

func (s *ObservedStore) UpsertTags(ctx context.Context, tags []Tag) error {
	tagStore, ok := s.base.(observedTagStore)
	if !ok {
		return nil
	}
	if err := tagStore.UpsertTags(ctx, tags); err != nil {
		return err
	}
	if len(tags) > 0 {
		s.tracker.Bump()
	}
	return nil
}

func (s *ObservedStore) UpdateTagSpecificity(ctx context.Context, name string, specificity, llm, embedding *float64, computedAt *time.Time) error {
	tagStore, ok := s.base.(observedTagStore)
	if !ok {
		return nil
	}
	if err := tagStore.UpdateTagSpecificity(ctx, name, specificity, llm, embedding, computedAt); err != nil {
		return err
	}
	s.tracker.Bump()
	return nil
}

func (s *ObservedStore) LoadMapProjectionData(ctx context.Context) ([]MapProjectionIssue, []Tag, error) {
	projectionStore, ok := s.base.(observedMapProjectionStore)
	if ok {
		return projectionStore.LoadMapProjectionData(ctx)
	}

	items, err := s.base.List(ctx)
	if err != nil {
		return nil, nil, err
	}

	detailed := make([]Issue, 0, len(items))
	for _, item := range items {
		issue, err := s.base.Get(ctx, item.ID)
		if err != nil {
			return nil, nil, err
		}
		detailed = append(detailed, issue)
	}

	tagStore, ok := s.base.(observedTagStore)
	if !ok {
		return MapProjectionIssuesFromIssues(detailed), nil, nil
	}
	tags, err := tagStore.ListTags(ctx)
	if err != nil {
		return nil, nil, err
	}
	return MapProjectionIssuesFromIssues(detailed), tags, nil
}

func (s *ObservedStore) ListIssueMetadata(ctx context.Context) ([]Issue, error) {
	metadataStore, ok := s.base.(observedIssueMetadataStore)
	if !ok {
		return s.base.List(ctx)
	}
	return metadataStore.ListIssueMetadata(ctx)
}

func (s *ObservedStore) ListIssueEmbeddingSimilarities(
	ctx context.Context,
	query []float64,
	limit int,
) ([]IssueEmbeddingSimilarity, int, float64, error) {
	similarityStore, ok := s.base.(observedIssueEmbeddingSimilarityStore)
	if !ok {
		return nil, 0, 0, nil
	}
	return similarityStore.ListIssueEmbeddingSimilarities(ctx, query, limit)
}

func (s *ObservedStore) ListPeopleAnalytics(
	ctx context.Context,
	opts ListOptions,
) ([]PeopleAnalyticsIssue, error) {
	analyticsStore, ok := s.base.(observedPeopleAnalyticsStore)
	if !ok {
		items, err := s.base.List(ctx)
		if err != nil {
			return nil, err
		}
		analytics := make([]PeopleAnalyticsIssue, 0, len(items))
		for _, item := range items {
			analytics = append(analytics, PeopleAnalyticsIssue{
				Status:     item.Status,
				AssignedTo: item.AssignedTo,
				TagScores:  append([]TagRelevance(nil), item.TagScores...),
				Embedding:  append([]float64(nil), item.Embedding...),
			})
		}
		return analytics, nil
	}
	return analyticsStore.ListPeopleAnalytics(ctx, opts)
}

func (s *ObservedStore) ListCompareIssues(ctx context.Context, ids []string) ([]CompareIssue, error) {
	compareStore, ok := s.base.(observedCompareIssueStore)
	if ok {
		return compareStore.ListCompareIssues(ctx, ids)
	}

	items := make([]CompareIssue, 0, len(ids))
	for _, id := range ids {
		issue, err := s.base.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, CompareIssue{
			ID:        issue.ID,
			Embedding: append([]float64(nil), issue.Embedding...),
		})
	}
	return items, nil
}
