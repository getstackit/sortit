package queries

import (
	"context"
	"strings"
	"time"

	"splat/internal/issues"
)

const defaultActivityLimit = 40

type ListActivity struct {
	Limit  int
	Cursor string
	Kind   string
}

type ActivityParticipant struct {
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
	Role       string `json:"role"`
}

type ActivityEvent struct {
	ID           string                `json:"id"`
	Kind         string                `json:"kind"`
	CreatedAt    time.Time             `json:"createdAt"`
	CreatedBy    string                `json:"createdBy"`
	EntityType   string                `json:"entityType"`
	EntityID     string                `json:"entityId"`
	Participants []ActivityParticipant `json:"participants,omitempty"`
	Body         string                `json:"body,omitempty"`
}

type ActivityResponse struct {
	Events     []ActivityEvent `json:"events"`
	NextCursor string          `json:"nextCursor,omitempty"`
}

type ListActivityHandler struct {
	Events issues.EventStore
}

func (h ListActivityHandler) Handle(ctx context.Context, input ListActivity) (ActivityResponse, error) {
	if h.Events == nil {
		return ActivityResponse{Events: []ActivityEvent{}}, nil
	}
	return h.handleFromEvents(ctx, input)
}

func (h ListActivityHandler) handleFromEvents(ctx context.Context, input ListActivity) (ActivityResponse, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = defaultActivityLimit
	}

	events, nextCursor, err := h.Events.ListEvents(ctx, limit, input.Cursor, strings.TrimSpace(input.Kind))
	if err != nil {
		return ActivityResponse{}, err
	}

	activityEvents := make([]ActivityEvent, 0, len(events))
	for _, event := range events {
		ae := ActivityEvent{
			ID:        event.ID,
			Kind:      event.Kind,
			CreatedAt: event.CreatedAt,
			CreatedBy: event.CreatedBy,
			Body:      event.Body,
		}

		if event.IssueID != "" {
			ae.EntityType = "issue"
			ae.EntityID = event.IssueID
		}

		if len(event.Participants) > 0 {
			ae.Participants = make([]ActivityParticipant, 0, len(event.Participants))
			for _, p := range event.Participants {
				ae.Participants = append(ae.Participants, ActivityParticipant{
					EntityType: "issue",
					EntityID:   p.IssueID,
					Role:       p.Role,
				})
			}
		}

		activityEvents = append(activityEvents, ae)
	}

	return ActivityResponse{
		Events:     activityEvents,
		NextCursor: nextCursor,
	}, nil
}
