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
	base    Store
	tracker *RevisionTracker
}

func NewObservedStore(base Store, tracker *RevisionTracker) *ObservedStore {
	if base == nil {
		return nil
	}
	return &ObservedStore{base: base, tracker: tracker}
}

func (s *ObservedStore) List(ctx context.Context) ([]Issue, error) {
	return s.base.List(ctx)
}

func (s *ObservedStore) Get(ctx context.Context, id string) (Issue, error) {
	return s.base.Get(ctx, id)
}

func (s *ObservedStore) Create(ctx context.Context, input CreateInput) (Issue, error) {
	issue, err := s.base.Create(ctx, input)
	if err == nil {
		s.tracker.Bump()
	}
	return issue, err
}

func (s *ObservedStore) Refine(ctx context.Context, id string, input RefineInput) (Issue, error) {
	issue, err := s.base.Refine(ctx, id, input)
	if err == nil {
		s.tracker.Bump()
	}
	return issue, err
}

func (s *ObservedStore) ProgressPost(ctx context.Context, id string, input ProgressInput) (Issue, error) {
	issue, err := s.base.ProgressPost(ctx, id, input)
	if err == nil {
		s.tracker.Bump()
	}
	return issue, err
}

func (s *ObservedStore) CloseIssue(ctx context.Context, id string, closedBy string) (Issue, error) {
	issue, err := s.base.CloseIssue(ctx, id, closedBy)
	if err == nil {
		s.tracker.Bump()
	}
	return issue, err
}

func (s *ObservedStore) ReopenIssue(ctx context.Context, id string) (Issue, error) {
	issue, err := s.base.ReopenIssue(ctx, id)
	if err == nil {
		s.tracker.Bump()
	}
	return issue, err
}

func (s *ObservedStore) AssignIssue(ctx context.Context, id, assignee string) (Issue, error) {
	issue, err := s.base.AssignIssue(ctx, id, assignee)
	if err == nil {
		s.tracker.Bump()
	}
	return issue, err
}

func (s *ObservedStore) SplitIssue(ctx context.Context, input SplitInput) (IssueOperationResult, error) {
	result, err := s.base.SplitIssue(ctx, input)
	if err == nil {
		s.tracker.Bump()
	}
	return result, err
}

func (s *ObservedStore) CombineIssues(ctx context.Context, input CombineInput) (IssueOperationResult, error) {
	result, err := s.base.CombineIssues(ctx, input)
	if err == nil {
		s.tracker.Bump()
	}
	return result, err
}

func (s *ObservedStore) LinkIssues(ctx context.Context, input LinkInput) (IssueOperationResult, error) {
	result, err := s.base.LinkIssues(ctx, input)
	if err == nil {
		s.tracker.Bump()
	}
	return result, err
}

type observedTagStore interface {
	ListTags(context.Context) ([]Tag, error)
	UpsertTags(context.Context, []Tag) error
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
