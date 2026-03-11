package issues

import (
	"context"
	"sync/atomic"
)

type RevisionTracker struct {
	revision atomic.Uint64
}

func NewRevisionTracker() *RevisionTracker {
	tracker := &RevisionTracker{}
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
	return t.revision.Add(1)
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

func (s *ObservedStore) List(ctx context.Context) ([]Issue, error) {
	return s.base.List(ctx)
}

func (s *ObservedStore) Get(ctx context.Context, id string) (Issue, error) {
	return s.base.Get(ctx, id)
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
}

type observedMapProjectionStore interface {
	LoadMapProjectionData(context.Context) ([]Issue, []Tag, error)
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

func (s *ObservedStore) LoadMapProjectionData(ctx context.Context) ([]Issue, []Tag, error) {
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
		return detailed, nil, nil
	}
	tags, err := tagStore.ListTags(ctx)
	if err != nil {
		return nil, nil, err
	}
	return detailed, tags, nil
}
