package queries

import (
	"context"

	"splat/internal/issues"
)

type GetIssueHandler struct {
	Store issues.Store
}

func (h GetIssueHandler) Handle(ctx context.Context, id string) (issues.Issue, error) {
	return h.Store.Get(ctx, id)
}
