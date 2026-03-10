package issues

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"splat/internal/issues/issuesdb"
)

func (s *PostgresStore) RecordEvent(ctx context.Context, event Event) error {
	participantsJSON, err := marshalJSONB(event.Participants, []EventParticipant{})
	if err != nil {
		return fmt.Errorf("marshal event participants: %w", err)
	}

	return s.queries.InsertEvent(ctx, issuesdb.InsertEventParams{
		ID:                strings.TrimSpace(event.ID),
		Kind:              strings.TrimSpace(event.Kind),
		IssueID:           strings.TrimSpace(event.IssueID),
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

	var rows []eventRow
	var err error

	switch {
	case kind != "" && hasCursor:
		dbRows, e := s.queries.ListEventsByKindBefore(ctx, issuesdb.ListEventsByKindBeforeParams{
			Kind:              kind,
			CreatedAtUnixNano: cursorTime.UnixNano(),
			ID:                cursorID,
			Limit:             fetchLimit,
		})
		err = e
		rows = convertEventsByKindBeforeRows(dbRows)
	case kind != "":
		dbRows, e := s.queries.ListEventsByKind(ctx, issuesdb.ListEventsByKindParams{
			Kind:  kind,
			Limit: fetchLimit,
		})
		err = e
		rows = convertEventsByKindRows(dbRows)
	case hasCursor:
		dbRows, e := s.queries.ListEventsBefore(ctx, issuesdb.ListEventsBeforeParams{
			CreatedAtUnixNano: cursorTime.UnixNano(),
			ID:                cursorID,
			Limit:             fetchLimit,
		})
		err = e
		rows = convertEventsBeforeRows(dbRows)
	default:
		dbRows, e := s.queries.ListEvents(ctx, fetchLimit)
		err = e
		rows = convertEventsRows(dbRows)
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

// eventRow is a unified type for all event query result shapes.
type eventRow struct {
	ID                string
	Kind              string
	IssueID           string
	CreatedBy         string
	CreatedAtUnixNano int64
	Body              string
	ParticipantsJSON  json.RawMessage
	IssueRaw          sql.NullString
	IssueStatus       sql.NullString
}

func convertEventsRows(rows []issuesdb.ListEventsRow) []eventRow {
	out := make([]eventRow, len(rows))
	for i, r := range rows {
		out[i] = eventRow{
			ID: r.ID, Kind: r.Kind, IssueID: r.IssueID,
			CreatedBy: r.CreatedBy, CreatedAtUnixNano: r.CreatedAtUnixNano,
			Body: r.Body, ParticipantsJSON: r.ParticipantsJson,
			IssueRaw: r.IssueRaw, IssueStatus: r.IssueStatus,
		}
	}
	return out
}

func convertEventsBeforeRows(rows []issuesdb.ListEventsBeforeRow) []eventRow {
	out := make([]eventRow, len(rows))
	for i, r := range rows {
		out[i] = eventRow{
			ID: r.ID, Kind: r.Kind, IssueID: r.IssueID,
			CreatedBy: r.CreatedBy, CreatedAtUnixNano: r.CreatedAtUnixNano,
			Body: r.Body, ParticipantsJSON: r.ParticipantsJson,
			IssueRaw: r.IssueRaw, IssueStatus: r.IssueStatus,
		}
	}
	return out
}

func convertEventsByKindRows(rows []issuesdb.ListEventsByKindRow) []eventRow {
	out := make([]eventRow, len(rows))
	for i, r := range rows {
		out[i] = eventRow{
			ID: r.ID, Kind: r.Kind, IssueID: r.IssueID,
			CreatedBy: r.CreatedBy, CreatedAtUnixNano: r.CreatedAtUnixNano,
			Body: r.Body, ParticipantsJSON: r.ParticipantsJson,
			IssueRaw: r.IssueRaw, IssueStatus: r.IssueStatus,
		}
	}
	return out
}

func convertEventsByKindBeforeRows(rows []issuesdb.ListEventsByKindBeforeRow) []eventRow {
	out := make([]eventRow, len(rows))
	for i, r := range rows {
		out[i] = eventRow{
			ID: r.ID, Kind: r.Kind, IssueID: r.IssueID,
			CreatedBy: r.CreatedBy, CreatedAtUnixNano: r.CreatedAtUnixNano,
			Body: r.Body, ParticipantsJSON: r.ParticipantsJson,
			IssueRaw: r.IssueRaw, IssueStatus: r.IssueStatus,
		}
	}
	return out
}

func eventFromRow(row eventRow) (Event, error) {
	var participants []EventParticipant
	if len(row.ParticipantsJSON) > 0 {
		if err := json.Unmarshal(row.ParticipantsJSON, &participants); err != nil {
			return Event{}, fmt.Errorf("unmarshal event participants: %w", err)
		}
	}
	if participants == nil {
		participants = []EventParticipant{}
	}

	return Event{
		ID:           row.ID,
		Kind:         row.Kind,
		IssueID:      row.IssueID,
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
