package queries

import (
	"context"
	"fmt"
	"strconv"
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
	Events    issues.EventStore
	ReadModel *ReadModelLoader // kept for fallback
}

func (h ListActivityHandler) Handle(ctx context.Context, input ListActivity) (ActivityResponse, error) {
	if h.Events != nil {
		return h.handleFromEvents(ctx, input)
	}
	return h.handleFromReadModel(ctx, input)
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

// handleFromReadModel is the legacy path using the read model (loads all issues).
func (h ListActivityHandler) handleFromReadModel(ctx context.Context, input ListActivity) (ActivityResponse, error) {
	model, err := h.ReadModel.Current(ctx)
	if err != nil {
		return ActivityResponse{}, err
	}

	events := buildActivityEvents(model.Corpus.Issues)
	events = filterActivityEvents(events, strings.TrimSpace(input.Kind))

	if input.Limit <= 0 {
		input.Limit = defaultActivityLimit
	}

	if cursorTime, cursorID, ok := decodeActivityCursor(input.Cursor); ok {
		filtered := events[:0]
		for _, event := range events {
			if event.CreatedAt.Before(cursorTime) || (event.CreatedAt.Equal(cursorTime) && event.ID < cursorID) {
				filtered = append(filtered, event)
			}
		}
		events = filtered
	}

	response := ActivityResponse{
		Events: events,
	}
	if len(events) > input.Limit {
		response.Events = append([]ActivityEvent(nil), events[:input.Limit]...)
		last := response.Events[len(response.Events)-1]
		response.NextCursor = encodeActivityCursor(last)
	}
	return response, nil
}

func buildActivityEvents(items []issues.Issue) []ActivityEvent {
	events := make([]ActivityEvent, 0)
	seenOperations := make(map[string]struct{})

	for _, item := range items {
		for _, post := range item.Discussion {
			kind := strings.TrimSpace(post.Kind)
			if kind == "" {
				if post.Sequence == 1 {
					kind = "report"
				} else {
					kind = "refinement"
				}
			}

			events = append(events, ActivityEvent{
				ID:         post.ID,
				Kind:       kind,
				CreatedAt:  post.CreatedAt,
				CreatedBy:  post.CreatedBy,
				EntityType: "issue",
				EntityID:   item.ID,
				Body:       post.Raw,
			})
		}

		for _, operation := range item.Operations {
			if _, seen := seenOperations[operation.ID]; seen {
				continue
			}
			seenOperations[operation.ID] = struct{}{}

			participants := make([]ActivityParticipant, 0, len(operation.Participants))
			for _, participant := range operation.Participants {
				participants = append(participants, ActivityParticipant{
					EntityType: "issue",
					EntityID:   participant.IssueID,
					Role:       participant.Role,
				})
			}

			events = append(events, ActivityEvent{
				ID:           operation.ID,
				Kind:         string(operation.Kind),
				CreatedAt:    operation.CreatedAt,
				CreatedBy:    operation.CreatedBy,
				Participants: participants,
				Body:         operation.Note,
			})
		}
	}

	// Sort by CreatedAt DESC, ID DESC
	for i := 1; i < len(events); i++ {
		for j := i; j > 0; j-- {
			if compareActivityEvents(events[j], events[j-1]) < 0 {
				events[j], events[j-1] = events[j-1], events[j]
			} else {
				break
			}
		}
	}

	return events
}

func compareActivityEvents(a, b ActivityEvent) int {
	if result := b.CreatedAt.Compare(a.CreatedAt); result != 0 {
		return result
	}
	if a.ID < b.ID {
		return 1
	}
	if a.ID > b.ID {
		return -1
	}
	return 0
}

func filterActivityEvents(events []ActivityEvent, kind string) []ActivityEvent {
	if kind == "" {
		return events
	}

	filtered := make([]ActivityEvent, 0, len(events))
	for _, event := range events {
		if strings.EqualFold(event.Kind, kind) {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func encodeActivityCursor(event ActivityEvent) string {
	return fmt.Sprintf("%d|%s", event.CreatedAt.UnixNano(), event.ID)
}

func decodeActivityCursor(value string) (time.Time, string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, "", false
	}

	parts := strings.SplitN(value, "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", false
	}

	unixNano, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, "", false
	}

	return time.Unix(0, unixNano).UTC(), parts[1], true
}
