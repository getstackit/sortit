package issues

import (
	"context"
	"time"
)

type Event struct {
	ID           string
	Kind         string // report, refinement, progress, closed, reopened, assigned, split, combine, link
	IssueID      string
	CreatedBy    string
	CreatedAt    time.Time
	Body         string
	Participants []EventParticipant
}

type EventParticipant struct {
	IssueID string `json:"issueId"`
	Role    string `json:"role"`
}

type EventStore interface {
	RecordEvent(ctx context.Context, event Event) error
	ListEvents(ctx context.Context, limit int, cursor string, kind string) ([]Event, string, error)
	ListLifecycleEvents(ctx context.Context, kinds []string, start, end time.Time) ([]Event, error)
}
