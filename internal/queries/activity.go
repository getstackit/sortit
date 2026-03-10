package queries

import (
	"context"
	"fmt"
	"slices"
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

type ActivityIssue struct {
	ID     string             `json:"id"`
	Raw    string             `json:"raw"`
	Status issues.IssueStatus `json:"status"`
}

type ActivityParticipant struct {
	IssueID string         `json:"issueId"`
	Role    string         `json:"role"`
	Issue   *ActivityIssue `json:"issue,omitempty"`
}

type ActivityEvent struct {
	ID           string                `json:"id"`
	Kind         string                `json:"kind"`
	CreatedAt    time.Time             `json:"createdAt"`
	CreatedBy    string                `json:"createdBy"`
	Issue        *ActivityIssue        `json:"issue,omitempty"`
	Participants []ActivityParticipant `json:"participants,omitempty"`
	Body         string                `json:"body,omitempty"`
}

type ActivityResponse struct {
	Events     []ActivityEvent `json:"events"`
	NextCursor string          `json:"nextCursor,omitempty"`
}

type ListActivityHandler struct {
	ReadModel *ReadModelLoader
}

func (h ListActivityHandler) Handle(ctx context.Context, input ListActivity) (ActivityResponse, error) {
	model, err := h.ReadModel.Current(ctx)
	if err != nil {
		return ActivityResponse{}, err
	}

	events := buildActivityEvents(model.Issues)
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
		issueRef := &ActivityIssue{
			ID:     item.ID,
			Raw:    item.Raw,
			Status: item.Status,
		}

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
				ID:        post.ID,
				Kind:      kind,
				CreatedAt: post.CreatedAt,
				CreatedBy: post.CreatedBy,
				Issue:     issueRef,
				Body:      post.Raw,
			})
		}

		for _, operation := range item.Operations {
			if _, seen := seenOperations[operation.ID]; seen {
				continue
			}
			seenOperations[operation.ID] = struct{}{}

			participants := make([]ActivityParticipant, 0, len(operation.Participants))
			for _, participant := range operation.Participants {
				entry := ActivityParticipant{
					IssueID: participant.IssueID,
					Role:    participant.Role,
				}
				if participant.Issue != nil {
					entry.Issue = &ActivityIssue{
						ID:     participant.Issue.ID,
						Raw:    participant.Issue.Raw,
						Status: participant.Issue.Status,
					}
				}
				participants = append(participants, entry)
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

	slices.SortFunc(events, func(a, b ActivityEvent) int {
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
	})

	return events
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
