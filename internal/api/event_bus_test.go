package api

import (
	"context"
	"errors"
	"testing"

	"sortit/internal/issues"
)

type recordingProjectionInvalidator struct {
	calls int
	err   error
}

func (i *recordingProjectionInvalidator) InvalidateMapProjections(context.Context) error {
	i.calls++
	return i.err
}

func TestProjectionInvalidationListenerInvalidatesOnEvent(t *testing.T) {
	t.Parallel()

	invalidator := &recordingProjectionInvalidator{}
	listener := projectionInvalidationListener(invalidator)
	if listener == nil {
		t.Fatal("expected projection invalidation listener")
	}

	listener(context.Background(), issues.Event{Kind: "report", IssueID: "issue-123"})

	if invalidator.calls != 1 {
		t.Fatalf("expected 1 invalidation call, got %d", invalidator.calls)
	}
}

func TestProjectionInvalidationListenerSkipsNonStructuralEvents(t *testing.T) {
	t.Parallel()

	invalidator := &recordingProjectionInvalidator{}
	listener := projectionInvalidationListener(invalidator)

	for _, kind := range []string{"assigned", "closed", "progress", "reopened"} {
		listener(context.Background(), issues.Event{Kind: kind, IssueID: "issue-123"})
	}

	if invalidator.calls != 0 {
		t.Fatalf("expected 0 invalidation calls, got %d", invalidator.calls)
	}
}

func TestProjectionInvalidationListenerReturnsNilWithoutInvalidator(t *testing.T) {
	t.Parallel()

	if listener := projectionInvalidationListener(nil); listener != nil {
		t.Fatal("expected nil listener without invalidator")
	}
}

func TestProjectionInvalidationListenerSwallowsInvalidationErrors(t *testing.T) {
	t.Parallel()

	invalidator := &recordingProjectionInvalidator{err: errors.New("boom")}
	listener := projectionInvalidationListener(invalidator)

	listener(context.Background(), issues.Event{Kind: "report", IssueID: "issue-123"})

	if invalidator.calls != 1 {
		t.Fatalf("expected 1 invalidation call, got %d", invalidator.calls)
	}
}
