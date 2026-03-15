package queries

import (
	"context"
	"log/slog"
	"time"

	"splat/internal/issues"
)

type GetIssueHandler struct {
	Store  issues.Store
	Logger *slog.Logger
}

func (h GetIssueHandler) Handle(ctx context.Context, id string) (issues.Issue, error) {
	start := time.Now()
	issue, err := h.Store.Get(ctx, id)
	if h.Logger != nil {
		h.Logger.InfoContext(ctx, "service call complete",
			"service", "GetIssueHandler.Handle",
			"issue_id", id,
			"duration", time.Since(start).Round(time.Millisecond),
		)
	}
	return issue, err
}
