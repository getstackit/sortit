package issues

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"splat/internal/issues/issuesdb"
)

type PostgresUnitOfWork struct {
	tx      *sql.Tx
	queries *issuesdb.Queries
}

func (s *PostgresStore) BeginUnitOfWork(ctx context.Context) (UnitOfWork, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin unit of work: %w", err)
	}
	return &PostgresUnitOfWork{
		tx:      tx,
		queries: s.queries.WithTx(tx),
	}, nil
}

func (u *PostgresUnitOfWork) Commit() error   { return u.tx.Commit() }
func (u *PostgresUnitOfWork) Rollback() error { return u.tx.Rollback() }

// Store reads

func (u *PostgresUnitOfWork) List(ctx context.Context) ([]Issue, error) {
	rows, err := u.queries.ListIssues(ctx)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	items := make([]Issue, 0, len(rows))
	for _, row := range rows {
		issue, err := issueFromQuery(issueModelFromListIssuesRow(row))
		if err != nil {
			return nil, err
		}
		items = append(items, issue)
	}
	states, err := loadIssueEnrichmentStates(ctx, u.tx, nil, issueIDs(items))
	if err != nil {
		return nil, err
	}
	items = applyIssueEnrichmentStates(items, states)
	return cloneIssues(items), nil
}

func (u *PostgresUnitOfWork) Get(ctx context.Context, id string) (Issue, error) {
	return u.GetIssueDetail(ctx, id)
}

func (u *PostgresUnitOfWork) GetIssueDetail(ctx context.Context, id string) (Issue, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Issue{}, ErrNotFound
	}

	issue, err := getIssueWithDiscussion(ctx, u.queries, id)
	if err != nil {
		return Issue{}, err
	}
	states, err := loadIssueEnrichmentStates(ctx, u.tx, nil, []string{id})
	if err != nil {
		return Issue{}, err
	}
	return applyIssueEnrichmentStates([]Issue{issue}, states)[0], nil
}

func (u *PostgresUnitOfWork) GetIssueDetailBase(ctx context.Context, id string) (Issue, error) {
	return getIssueDetailBase(ctx, u.queries, id)
}

func (u *PostgresUnitOfWork) ListIssueDetailPosts(ctx context.Context, id string) ([]IssuePost, error) {
	return listIssueDetailPosts(ctx, u.queries, id)
}

func (u *PostgresUnitOfWork) ListIssueDetailLinks(ctx context.Context, id string) ([]IssueLink, error) {
	return listIssueDetailLinks(ctx, u.queries, id)
}

func (u *PostgresUnitOfWork) ListIssueDetailOperations(ctx context.Context, id string) ([]IssueOperation, error) {
	return listIssueDetailOperations(ctx, u.queries, id)
}

func (u *PostgresUnitOfWork) ListIssueDetailReferences(ctx context.Context, ids []string) ([]IssueReference, error) {
	return listIssueDetailReferences(ctx, u.queries, ids)
}

func (u *PostgresUnitOfWork) ListIssueSnapshots(ctx context.Context, id string) ([]IssueSnapshot, error) {
	return listIssueSnapshots(ctx, u.tx, id)
}

// Store writes

func (u *PostgresUnitOfWork) SaveIssue(ctx context.Context, issue Issue) error {
	record, err := recordFromIssue(issue)
	if err != nil {
		return err
	}

	if err := u.queries.InsertIssue(ctx, issuesdb.InsertIssueParams{
		ID:                record.ID,
		Raw:               record.Raw,
		TagsJson:          record.TagsJSON,
		CreatedBy:         record.CreatedBy,
		CreatedAtUnixNano: record.CreatedAtUnixNano,
		Status:            record.Status,
		ClosedAtUnixNano:  record.ClosedAtUnixNano,
		ClosedBy:          record.ClosedBy,
		TagScoresJson:     record.TagScoresJSON,
		Column10:          record.EmbeddingVector,
		AssignedTo:        record.AssignedTo,
	}); err != nil {
		return fmt.Errorf("save issue: %w", err)
	}
	if err := updateIssueEnrichmentState(ctx, u.tx, record.ID, issueFieldUpdateForIssue(issue)); err != nil {
		return err
	}
	if err := insertIssuePosts(ctx, u.queries, issue.Discussion); err != nil {
		return err
	}
	return nil
}

func (u *PostgresUnitOfWork) SaveIssuePost(ctx context.Context, post IssuePost) error {
	if err := u.queries.InsertIssuePost(ctx, issuesdb.InsertIssuePostParams{
		ID:                strings.TrimSpace(post.ID),
		IssueID:           strings.TrimSpace(post.IssueID),
		Raw:               strings.TrimSpace(post.Raw),
		CreatedBy:         strings.TrimSpace(post.CreatedBy),
		CreatedAtUnixNano: post.CreatedAt.UTC().UnixNano(),
		Sequence:          int64(post.Sequence),
		Kind:              post.Kind,
	}); err != nil {
		return fmt.Errorf("save issue post: %w", err)
	}
	return nil
}

func (u *PostgresUnitOfWork) UpdateIssueFields(ctx context.Context, id string, fields IssueFieldUpdate) error {
	id = strings.TrimSpace(id)

	if fields.Status != nil && *fields.Status == StatusClosed && fields.ClosedAt != nil && fields.ClosedBy != nil {
		if err := u.queries.CloseIssue(ctx, issuesdb.CloseIssueParams{
			ID:               id,
			ClosedAtUnixNano: fields.ClosedAt.UTC().UnixNano(),
			ClosedBy:         *fields.ClosedBy,
		}); err != nil {
			return fmt.Errorf("close issue fields: %w", err)
		}
	}

	if fields.Status != nil && *fields.Status == StatusOpen {
		if err := u.queries.ReopenIssue(ctx, id); err != nil {
			return fmt.Errorf("reopen issue fields: %w", err)
		}
	}

	if fields.AssignedTo != nil {
		if err := u.queries.AssignIssue(ctx, issuesdb.AssignIssueParams{
			AssignedTo: *fields.AssignedTo,
			ID:         id,
		}); err != nil {
			return fmt.Errorf("assign issue fields: %w", err)
		}
	}

	if fields.Raw != nil {
		record, err := buildRefinementRecord(id, fields)
		if err != nil {
			return err
		}
		if err := u.queries.UpdateIssueRefinement(ctx, record); err != nil {
			return fmt.Errorf("update issue refinement fields: %w", err)
		}
		if snapshot, ok := issueSnapshotFromFieldUpdate(id, fields); ok {
			if err := saveIssueSnapshot(ctx, u.tx, snapshot); err != nil {
				return err
			}
		}
	}
	if err := updateIssueEnrichmentState(ctx, u.tx, id, fields); err != nil {
		return err
	}

	return nil
}

func (u *PostgresUnitOfWork) SaveOperation(ctx context.Context, op IssueOperation) error {
	if err := insertIssueOperation(ctx, u.queries, op); err != nil {
		return err
	}
	for i, p := range op.Participants {
		if err := insertIssueOperationParticipant(ctx, u.queries, op.ID, p, i+1); err != nil {
			return err
		}
	}
	return nil
}

func (u *PostgresUnitOfWork) SaveLink(ctx context.Context, link IssueLink) error {
	return insertIssueLink(ctx, u.queries, link)
}

// EventStore

func (u *PostgresUnitOfWork) RecordEvent(ctx context.Context, event Event) error {
	participantsJSON, err := marshalJSONB(event.Participants, []EventParticipant{})
	if err != nil {
		return fmt.Errorf("marshal event participants: %w", err)
	}

	issueID := strings.TrimSpace(event.IssueID)

	return u.queries.InsertEvent(ctx, issuesdb.InsertEventParams{
		ID:                strings.TrimSpace(event.ID),
		Kind:              strings.TrimSpace(event.Kind),
		IssueID:           sql.NullString{String: issueID, Valid: issueID != ""},
		CreatedBy:         strings.TrimSpace(event.CreatedBy),
		CreatedAtUnixNano: event.CreatedAt.UTC().UnixNano(),
		Body:              event.Body,
		ParticipantsJson:  participantsJSON,
	})
}

func (u *PostgresUnitOfWork) ListEvents(ctx context.Context, limit int, cursor string, kind string) ([]Event, string, error) {
	if limit <= 0 {
		limit = 40
	}
	fetchLimit := int32(limit + 1)

	kind = strings.TrimSpace(kind)
	cursorTime, cursorID, hasCursor := decodeEventCursor(cursor)

	var rows []issuesdb.Event
	var err error

	switch {
	case kind != "" && hasCursor:
		rows, err = u.queries.ListEventsByKindBefore(ctx, issuesdb.ListEventsByKindBeforeParams{
			Kind:              kind,
			CreatedAtUnixNano: cursorTime.UnixNano(),
			ID:                cursorID,
			Limit:             fetchLimit,
		})
	case kind != "":
		rows, err = u.queries.ListEventsByKind(ctx, issuesdb.ListEventsByKindParams{
			Kind:  kind,
			Limit: fetchLimit,
		})
	case hasCursor:
		rows, err = u.queries.ListEventsBefore(ctx, issuesdb.ListEventsBeforeParams{
			CreatedAtUnixNano: cursorTime.UnixNano(),
			ID:                cursorID,
			Limit:             fetchLimit,
		})
	default:
		rows, err = u.queries.ListEvents(ctx, fetchLimit)
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
		nextCursor = fmt.Sprintf("%d|%s", last.CreatedAt.UnixNano(), last.ID)
	}

	return events, nextCursor, nil
}
