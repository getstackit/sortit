package issues

import "context"

// UnitOfWork groups store and event writes into a single atomic operation.
// For PostgresStore this maps to a database transaction; for InMemoryStore
// it is a passthrough.
type UnitOfWork interface {
	Store
	EventStore
	Commit() error
	Rollback() error
}

// UnitOfWorkBeginner opens a new UnitOfWork.
type UnitOfWorkBeginner interface {
	BeginUnitOfWork(ctx context.Context) (UnitOfWork, error)
}
