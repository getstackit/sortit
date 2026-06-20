package curation

import (
	"context"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/issues/commands"
	"sortit/internal/memories"
)

// Dispatcher applies an accepted curation move by invoking the underlying
// command handler. It is an interface so the service stays testable without the
// full handler/UnitOfWork wiring.
type Dispatcher interface {
	Combine(ctx context.Context, in commands.CombineIssues) (issues.IssueOperationResult, error)
	Close(ctx context.Context, in commands.CloseIssue) (issues.Issue, error)
	Reenrich(ctx context.Context, in commands.ReEnrichIssue) (issues.Issue, error)
	SupersedeMemory(ctx context.Context, id, supersededBy string) (domain.Memory, error)
	ArchiveMemory(ctx context.Context, id string) (domain.Memory, error)
}

// HandlerDispatcher adapts the already-constructed command handlers and memory
// service into the Dispatcher interface. The handlers each manage their own
// unit of work, so dispatch is a thin pass-through.
type HandlerDispatcher struct {
	CombineHandler  commands.CombineIssuesHandler
	CloseHandler    commands.CloseIssueHandler
	ReenrichHandler commands.ReEnrichIssueHandler
	Memories        *memories.Service
}

func (d HandlerDispatcher) Combine(ctx context.Context, in commands.CombineIssues) (issues.IssueOperationResult, error) {
	return d.CombineHandler.Handle(ctx, in)
}

func (d HandlerDispatcher) Close(ctx context.Context, in commands.CloseIssue) (issues.Issue, error) {
	return d.CloseHandler.Handle(ctx, in)
}

func (d HandlerDispatcher) Reenrich(ctx context.Context, in commands.ReEnrichIssue) (issues.Issue, error) {
	return d.ReenrichHandler.Handle(ctx, in)
}

func (d HandlerDispatcher) SupersedeMemory(ctx context.Context, id, supersededBy string) (domain.Memory, error) {
	return d.Memories.SupersedeMemory(ctx, id, supersededBy)
}

func (d HandlerDispatcher) ArchiveMemory(ctx context.Context, id string) (domain.Memory, error) {
	return d.Memories.ArchiveMemory(ctx, id)
}
