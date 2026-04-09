package commands

import (
	"context"
	"fmt"
	"strings"

	"sortit/internal/issues"
)

// CommandRunner manages the database transaction lifecycle for command execution.
type CommandRunner struct {
	DB       issues.UnitOfWorkBeginner
	OnCommit func() // called after a successful commit; may be nil
}

// Begin opens a unit of work and returns a finisher suitable for defer.
// The finisher rolls back on error and commits on success.
func Begin(
	ctx context.Context,
	runner *CommandRunner,
) (issues.UnitOfWork, func(*error), error) {
	uow, err := runner.DB.BeginUnitOfWork(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin transaction: %w", err)
	}

	finish := func(errp *error) {
		if errp != nil && *errp != nil {
			_ = uow.Rollback()
			return
		}

		if err := uow.Commit(); err != nil {
			if errp != nil {
				*errp = fmt.Errorf("commit transaction: %w", err)
			}
			_ = uow.Rollback()
			return
		}

		if runner.OnCommit != nil {
			runner.OnCommit()
		}
	}

	return uow, finish, nil
}

func FinishAndThen(errp *error, finish func(*error), afterSuccess func()) {
	if finish != nil {
		finish(errp)
	}
	if errp != nil && *errp == nil && afterSuccess != nil {
		afterSuccess()
	}
}

func FinishAndPublish(
	errp *error,
	finish func(*error),
	ctx context.Context,
	publisher issues.EventPublisher,
	events ...issues.Event,
) {
	FinishAndThen(errp, finish, func() {
		if publisher == nil {
			return
		}
		for _, event := range events {
			if strings.TrimSpace(event.Kind) == "" {
				continue
			}
			publisher.PublishOne(ctx, event)
		}
	})
}
