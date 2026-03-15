package issues

import "context"

// InMemoryUnitOfWork wraps InMemoryStore with no-op commit/rollback.
type InMemoryUnitOfWork struct {
	*InMemoryStore
}

func (s *InMemoryStore) BeginUnitOfWork(_ context.Context) (UnitOfWork, error) {
	return &InMemoryUnitOfWork{InMemoryStore: s}, nil
}

func (u *InMemoryUnitOfWork) Commit() error   { return nil }
func (u *InMemoryUnitOfWork) Rollback() error { return nil }
