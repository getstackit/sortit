package commands

import (
	"context"
	"fmt"

	"splat/internal/issues"
)

// CommandRunner manages the database transaction lifecycle for command execution.
type CommandRunner struct {
	DB       issues.UnitOfWorkBeginner
	OnCommit func() // called after a successful commit; may be nil
}

// Run executes fn inside a unit of work (database transaction).
// It commits on success and rolls back on error.
func Run[T any](ctx context.Context, runner *CommandRunner, fn func(ctx context.Context, uow issues.UnitOfWork) (T, error)) (T, error) {
	uow, err := runner.DB.BeginUnitOfWork(ctx)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("begin transaction: %w", err)
	}
	defer uow.Rollback()

	result, err := fn(ctx, uow)
	if err != nil {
		var zero T
		return zero, err
	}

	if err := uow.Commit(); err != nil {
		var zero T
		return zero, fmt.Errorf("commit transaction: %w", err)
	}

	if runner.OnCommit != nil {
		runner.OnCommit()
	}

	return result, nil
}
