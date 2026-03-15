package queries

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"splat/internal/issues"
)

type GetIssue struct {
	ID string
}

type GetIssueHandler struct {
	Reader issues.Reader
	Logger *slog.Logger
}

func (h GetIssueHandler) Handle(ctx context.Context, input GetIssue) (issues.Issue, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return issues.Issue{}, issues.ErrNotFound
	}

	start := time.Now()
	issue, err := h.Reader.Get(ctx, id)
	if h.Logger != nil {
		h.Logger.InfoContext(ctx, "service call complete",
			"service", "GetIssueHandler.Handle",
			"issue_id", id,
			"duration", time.Since(start).Round(time.Millisecond),
		)
	}
	return issue, err
}
