package issues

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"sortit/internal/issues/issuesdb"
)

func (s *PostgresStore) RecordEvent(ctx context.Context, event Event) error {
	participantsJSON, err := marshalJSONB(event.Participants, []EventParticipant{})
	if err != nil {
		return fmt.Errorf("marshal event participants: %w", err)
	}

	issueID := strings.TrimSpace(event.IssueID)

	return s.queries.InsertEvent(ctx, issuesdb.InsertEventParams{
		ID:                strings.TrimSpace(event.ID),
		Kind:              strings.TrimSpace(event.Kind),
		IssueID:           sql.NullString{String: issueID, Valid: issueID != ""},
		CreatedBy:         strings.TrimSpace(event.CreatedBy),
		CreatedAtUnixNano: event.CreatedAt.UTC().UnixNano(),
		Body:              event.Body,
		ParticipantsJson:  participantsJSON,
	})
}

func (s *PostgresStore) ListEvents(ctx context.Context, limit int, cursor string, kind string) ([]Event, string, error) {
	if limit <= 0 {
		limit = 40
	}
	// Fetch one extra to determine if there's a next page.
	fetchLimit := int32(limit + 1)

	kind = strings.TrimSpace(kind)
	cursorTime, cursorID, hasCursor := decodeEventCursor(cursor)

	var rows []issuesdb.Event
	var err error

	switch {
	case kind != "" && hasCursor:
		rows, err = s.queries.ListEventsByKindBefore(ctx, issuesdb.ListEventsByKindBeforeParams{
			Kind:              kind,
			CreatedAtUnixNano: cursorTime.UnixNano(),
			ID:                cursorID,
			Limit:             fetchLimit,
		})
	case kind != "":
		rows, err = s.queries.ListEventsByKind(ctx, issuesdb.ListEventsByKindParams{
			Kind:  kind,
			Limit: fetchLimit,
		})
	case hasCursor:
		rows, err = s.queries.ListEventsBefore(ctx, issuesdb.ListEventsBeforeParams{
			CreatedAtUnixNano: cursorTime.UnixNano(),
			ID:                cursorID,
			Limit:             fetchLimit,
		})
	default:
		rows, err = s.queries.ListEvents(ctx, fetchLimit)
	}

	if err != nil {
		return nil, "", fmt.Errorf("list events: %w", err)
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	events := make([]Event, 0, len(rows))
	for _, row := range rows {
		event, err := eventFromRow(row)
		if err != nil {
			return nil, "", err
		}
		events = append(events, event)
	}

	var nextCursor string
	if hasMore && len(events) > 0 {
		last := events[len(events)-1]
		nextCursor = encodeEventCursor(last)
	}

	return events, nextCursor, nil
}

// ListLifecycleEvents returns all events whose kind matches one of the
// given kinds and whose created_at falls in [start, end]. Empty kinds
// short-circuit to nil. Used by region churn computation.
func (s *PostgresStore) ListLifecycleEvents(ctx context.Context, kinds []string, start, end time.Time) ([]Event, error) {
	if len(kinds) == 0 {
		return nil, nil
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, kind, issue_id, created_by, created_at_unix_nano, body, participants_json
		 FROM events
		 WHERE kind = ANY($1::text[])
		   AND created_at_unix_nano BETWEEN $2 AND $3
		   AND issue_id != ''
		 ORDER BY created_at_unix_nano ASC, id ASC`,
		pq.Array(kinds), start.UnixNano(), end.UnixNano(),
	)
	if err != nil {
		return nil, fmt.Errorf("list lifecycle events: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	out := make([]Event, 0)
	for rows.Next() {
		var (
			id           string
			kind         string
			issueID      string
			createdBy    string
			createdAtNS  int64
			body         string
			participants []byte
		)
		if err := rows.Scan(&id, &kind, &issueID, &createdBy, &createdAtNS, &body, &participants); err != nil {
			return nil, fmt.Errorf("scan lifecycle event: %w", err)
		}
		event := Event{
			ID:        id,
			Kind:      kind,
			IssueID:   issueID,
			CreatedBy: createdBy,
			CreatedAt: time.Unix(0, createdAtNS).UTC(),
			Body:      body,
		}
		if len(participants) > 0 {
			if err := json.Unmarshal(participants, &event.Participants); err != nil {
				return nil, fmt.Errorf("unmarshal lifecycle event participants: %w", err)
			}
		}
		out = append(out, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lifecycle events: %w", err)
	}
	return out, nil
}

func eventFromRow(row issuesdb.Event) (Event, error) {
	var participants []EventParticipant
	if len(row.ParticipantsJson) > 0 {
		if err := json.Unmarshal(row.ParticipantsJson, &participants); err != nil {
			return Event{}, fmt.Errorf("unmarshal event participants: %w", err)
		}
	}
	if participants == nil {
		participants = []EventParticipant{}
	}

	return Event{
		ID:           row.ID,
		Kind:         row.Kind,
		IssueID:      row.IssueID.String,
		CreatedBy:    row.CreatedBy,
		CreatedAt:    time.Unix(0, row.CreatedAtUnixNano).UTC(),
		Body:         row.Body,
		Participants: participants,
	}, nil
}

func encodeEventCursor(event Event) string {
	return fmt.Sprintf("%d|%s", event.CreatedAt.UnixNano(), event.ID)
}

func decodeEventCursor(value string) (time.Time, string, bool) {
	return decodeActivityCursorValue(value)
}

func decodeActivityCursorValue(value string) (time.Time, string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, "", false
	}

	parts := strings.SplitN(value, "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", false
	}

	var unixNano int64
	for _, ch := range parts[0] {
		if ch < '0' || ch > '9' {
			return time.Time{}, "", false
		}
		unixNano = unixNano*10 + int64(ch-'0')
	}

	return time.Unix(0, unixNano).UTC(), parts[1], true
}
