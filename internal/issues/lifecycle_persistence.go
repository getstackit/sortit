package issues

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"sortit/internal/issues/issuesdb"
)

type lifecycleProjectionState struct {
	IssueID          string
	CreatedBy        string
	CreatedAt        time.Time
	Status           IssueStatus
	ClosedAt         *time.Time
	ClosedBy         string
	ClosedReason     string
	ClosedReasonNote string
	AssignedTo       string
	FactCount        int64
}

type lifecycleFactRecord struct {
	ID        string
	IssueID   string
	Sequence  int64
	Kind      string
	CreatedBy string
	CreatedAt time.Time
	Payload   json.RawMessage
	Source    string
	SourceID  string
	Inferred  bool
}

func syncIssueLifecycleOnCreate(ctx context.Context, db issuesdb.DBTX, issue Issue) error {
	state := lifecycleProjectionState{
		IssueID:    strings.TrimSpace(issue.ID),
		CreatedBy:  strings.TrimSpace(issue.CreatedBy),
		CreatedAt:  issue.CreatedAt.UTC(),
		Status:     normalizeIssueStatus(issue.Status),
		AssignedTo: strings.TrimSpace(issue.AssignedTo),
	}
	if state.Status == "" {
		state.Status = StatusOpen
	}
	if issue.ClosedAt != nil {
		closedAt := issue.ClosedAt.UTC()
		state.ClosedAt = &closedAt
	}
	state.ClosedBy = strings.TrimSpace(issue.ClosedBy)
	state.ClosedReason = strings.TrimSpace(issue.ClosedReason)
	state.ClosedReasonNote = strings.TrimSpace(issue.ClosedReasonNote)

	initialPostID := state.IssueID + ":created"
	if discussion := initialDiscussion(issue); len(discussion) > 0 && strings.TrimSpace(discussion[0].ID) != "" {
		initialPostID = strings.TrimSpace(discussion[0].ID)
	}

	sequence := int64(1)
	createdFact := lifecycleFactRecord{
		ID:        NewLifecycleFactID(),
		IssueID:   state.IssueID,
		Sequence:  sequence,
		Kind:      "created",
		CreatedBy: state.CreatedBy,
		CreatedAt: state.CreatedAt,
		Payload: mustLifecycleJSON(map[string]any{
			"raw": strings.TrimSpace(issue.Raw),
		}),
		Source:   "issue_create",
		SourceID: initialPostID,
	}
	if err := insertLifecycleFact(ctx, db, createdFact); err != nil {
		return err
	}
	lastFactID := createdFact.ID
	state.FactCount = 1

	if state.AssignedTo != "" {
		sequence++
		assignedFact := lifecycleFactRecord{
			ID:        NewLifecycleFactID(),
			IssueID:   state.IssueID,
			Sequence:  sequence,
			Kind:      "assigned",
			CreatedBy: DefaultActor(state.CreatedBy),
			CreatedAt: state.CreatedAt,
			Payload: mustLifecycleJSON(map[string]any{
				"assignedTo": state.AssignedTo,
			}),
			Source:   "issue_create",
			SourceID: state.IssueID + ":assigned",
		}
		if err := insertLifecycleFact(ctx, db, assignedFact); err != nil {
			return err
		}
		lastFactID = assignedFact.ID
		state.FactCount++
	}

	if state.Status == StatusClosed {
		sequence++
		closedAt := state.CreatedAt
		if state.ClosedAt != nil {
			closedAt = state.ClosedAt.UTC()
		}
		closedFact := lifecycleFactRecord{
			ID:        NewLifecycleFactID(),
			IssueID:   state.IssueID,
			Sequence:  sequence,
			Kind:      "closed",
			CreatedBy: DefaultActor(state.ClosedBy),
			CreatedAt: closedAt,
			Payload: mustLifecycleJSON(map[string]any{
				"closedAtUnixNano": closedAt.UnixNano(),
				"closedBy":         DefaultActor(state.ClosedBy),
				"reason":           state.ClosedReason,
				"reasonNote":       state.ClosedReasonNote,
			}),
			Source:   "issue_create",
			SourceID: state.IssueID + ":closed",
		}
		if err := insertLifecycleFact(ctx, db, closedFact); err != nil {
			return err
		}
		lastFactID = closedFact.ID
		state.FactCount++
	}

	return upsertLifecycleProjection(ctx, db, state, lastFactID)
}

func applyLifecycleIssueFieldUpdate(ctx context.Context, db issuesdb.DBTX, issueID string, fields IssueFieldUpdate) error {
	if !hasLifecycleFieldChanges(fields) {
		return nil
	}

	state, err := loadLifecycleProjectionState(ctx, db, issueID)
	if err != nil {
		return err
	}
	sequence, err := nextLifecycleFactSequence(ctx, db, issueID)
	if err != nil {
		return err
	}

	lastFactID := ""

	if fields.Status != nil && *fields.Status == StatusClosed && fields.ClosedAt != nil && fields.ClosedBy != nil {
		closedAt := fields.ClosedAt.UTC()
		closedBy := DefaultActor(derefString(fields.ClosedBy))
		fact := lifecycleFactRecord{
			ID:        NewLifecycleFactID(),
			IssueID:   issueID,
			Sequence:  sequence,
			Kind:      "closed",
			CreatedBy: closedBy,
			CreatedAt: closedAt,
			Payload: mustLifecycleJSON(map[string]any{
				"closedAtUnixNano": closedAt.UnixNano(),
				"closedBy":         closedBy,
				"reason":           derefString(fields.ClosedReason),
				"reasonNote":       derefString(fields.ClosedReasonNote),
			}),
			Source:   "issue_update",
			SourceID: NewLifecycleFactID(),
		}
		if err := insertLifecycleFact(ctx, db, fact); err != nil {
			return err
		}
		lastFactID = fact.ID
		state.Status = StatusClosed
		state.ClosedAt = &closedAt
		state.ClosedBy = closedBy
		state.ClosedReason = strings.TrimSpace(derefString(fields.ClosedReason))
		state.ClosedReasonNote = strings.TrimSpace(derefString(fields.ClosedReasonNote))
		state.FactCount++
		sequence++
	}

	if fields.Status != nil && *fields.Status == StatusOpen {
		reopenedAt := lifecycleFactTimestamp(fields)
		reopenedBy := DefaultActor(derefString(fields.LifecycleCreatedBy))
		fact := lifecycleFactRecord{
			ID:        NewLifecycleFactID(),
			IssueID:   issueID,
			Sequence:  sequence,
			Kind:      "reopened",
			CreatedBy: reopenedBy,
			CreatedAt: reopenedAt,
			Payload:   mustLifecycleJSON(map[string]any{}),
			Source:    "issue_update",
			SourceID:  NewLifecycleFactID(),
		}
		if err := insertLifecycleFact(ctx, db, fact); err != nil {
			return err
		}
		lastFactID = fact.ID
		state.Status = StatusOpen
		state.ClosedAt = nil
		state.ClosedBy = ""
		state.ClosedReason = ""
		state.ClosedReasonNote = ""
		state.FactCount++
		sequence++
	}

	if fields.AssignedTo != nil {
		assignedAt := lifecycleFactTimestamp(fields)
		assignedBy := DefaultActor(derefString(fields.LifecycleCreatedBy))
		assignedTo := strings.TrimSpace(*fields.AssignedTo)
		fact := lifecycleFactRecord{
			ID:        NewLifecycleFactID(),
			IssueID:   issueID,
			Sequence:  sequence,
			Kind:      "assigned",
			CreatedBy: assignedBy,
			CreatedAt: assignedAt,
			Payload: mustLifecycleJSON(map[string]any{
				"assignedTo": assignedTo,
			}),
			Source:   "issue_update",
			SourceID: NewLifecycleFactID(),
		}
		if err := insertLifecycleFact(ctx, db, fact); err != nil {
			return err
		}
		lastFactID = fact.ID
		state.AssignedTo = assignedTo
		state.FactCount++
	}

	if lastFactID == "" {
		return nil
	}
	return upsertLifecycleProjection(ctx, db, state, lastFactID)
}

func hasLifecycleFieldChanges(fields IssueFieldUpdate) bool {
	if fields.AssignedTo != nil {
		return true
	}
	if fields.Status == nil {
		return false
	}
	switch *fields.Status {
	case StatusClosed:
		return fields.ClosedAt != nil && fields.ClosedBy != nil
	case StatusOpen:
		return true
	default:
		return false
	}
}

func lifecycleFactTimestamp(fields IssueFieldUpdate) time.Time {
	if fields.LifecycleCreatedAt != nil && !fields.LifecycleCreatedAt.IsZero() {
		return fields.LifecycleCreatedAt.UTC()
	}
	return time.Now().UTC()
}

func loadLifecycleProjectionState(ctx context.Context, db issuesdb.DBTX, issueID string) (lifecycleProjectionState, error) {
	var (
		state             lifecycleProjectionState
		createdAtUnixNano int64
		closedAtUnixNano  int64
		factCount         int64
		status            string
	)
	if err := db.QueryRowContext(
		ctx,
		`SELECT i.id,
		        COALESCE(p.created_by, i.created_by) AS created_by,
		        COALESCE(p.created_at_unix_nano, i.created_at_unix_nano) AS created_at_unix_nano,
		        COALESCE(p.status, i.status) AS status,
		        COALESCE(p.closed_at_unix_nano, 0) AS closed_at_unix_nano,
		        COALESCE(p.closed_by, '') AS closed_by,
		        COALESCE(p.closed_reason, '') AS closed_reason,
		        COALESCE(p.closed_reason_note, '') AS closed_reason_note,
		        COALESCE(p.assigned_to, i.assigned_to) AS assigned_to,
		        COALESCE(p.fact_count, 0) AS fact_count
		 FROM issues i
		 LEFT JOIN issue_lifecycle_projections p ON p.issue_id = i.id
		 WHERE i.id = $1`,
		issueID,
	).Scan(
		&state.IssueID,
		&state.CreatedBy,
		&createdAtUnixNano,
		&status,
		&closedAtUnixNano,
		&state.ClosedBy,
		&state.ClosedReason,
		&state.ClosedReasonNote,
		&state.AssignedTo,
		&factCount,
	); err != nil {
		return lifecycleProjectionState{}, fmt.Errorf("load lifecycle projection state for %q: %w", issueID, err)
	}

	state.CreatedAt = time.Unix(0, createdAtUnixNano).UTC()
	state.Status = normalizeIssueStatus(IssueStatus(status))
	if closedAtUnixNano > 0 {
		closedAt := time.Unix(0, closedAtUnixNano).UTC()
		state.ClosedAt = &closedAt
	}
	state.FactCount = factCount
	return state, nil
}

func nextLifecycleFactSequence(ctx context.Context, db issuesdb.DBTX, issueID string) (int64, error) {
	var sequence int64
	if err := db.QueryRowContext(
		ctx,
		`SELECT COALESCE(MAX(sequence), 0) + 1
		 FROM issue_lifecycle_facts
		 WHERE issue_id = $1`,
		issueID,
	).Scan(&sequence); err != nil {
		return 0, fmt.Errorf("load next lifecycle fact sequence for %q: %w", issueID, err)
	}
	return sequence, nil
}

func insertLifecycleFact(ctx context.Context, db issuesdb.DBTX, fact lifecycleFactRecord) error {
	payload, err := json.Marshal(fact.Payload)
	if err != nil {
		return fmt.Errorf("marshal lifecycle fact payload for %q: %w", fact.ID, err)
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO issue_lifecycle_facts (
		     id,
		     issue_id,
		     sequence,
		     kind,
		     created_by,
		     created_at_unix_nano,
		     payload_json,
		     source,
		     source_id,
		     inferred
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10)`,
		strings.TrimSpace(fact.ID),
		strings.TrimSpace(fact.IssueID),
		fact.Sequence,
		strings.TrimSpace(fact.Kind),
		strings.TrimSpace(fact.CreatedBy),
		fact.CreatedAt.UTC().UnixNano(),
		payload,
		strings.TrimSpace(fact.Source),
		strings.TrimSpace(fact.SourceID),
		fact.Inferred,
	); err != nil {
		return fmt.Errorf("insert lifecycle fact %q: %w", fact.ID, err)
	}
	return nil
}

func upsertLifecycleProjection(
	ctx context.Context,
	db issuesdb.DBTX,
	state lifecycleProjectionState,
	lastFactID string,
) error {
	// Open issues store NULL for the closed_* columns; only closed issues carry
	// values (issue_lifecycle_projections.closed_* is nullable as of 000032).
	var (
		closedAtUnixNano any
		closedBy         any
		closedReason     any
		closedReasonNote any
	)
	if state.ClosedAt != nil {
		closedAtUnixNano = state.ClosedAt.UTC().UnixNano()
		closedBy = strings.TrimSpace(state.ClosedBy)
		closedReason = strings.TrimSpace(state.ClosedReason)
		closedReasonNote = strings.TrimSpace(state.ClosedReasonNote)
	}

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO issue_lifecycle_projections (
		     issue_id,
		     created_by,
		     created_at_unix_nano,
		     status,
		     closed_at_unix_nano,
		     closed_by,
		     closed_reason,
		     closed_reason_note,
		     assigned_to,
		     last_fact_id,
		     fact_count,
		     updated_at_unix_nano
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (issue_id) DO UPDATE
		 SET created_by = EXCLUDED.created_by,
		     created_at_unix_nano = EXCLUDED.created_at_unix_nano,
		     status = EXCLUDED.status,
		     closed_at_unix_nano = EXCLUDED.closed_at_unix_nano,
		     closed_by = EXCLUDED.closed_by,
		     closed_reason = EXCLUDED.closed_reason,
		     closed_reason_note = EXCLUDED.closed_reason_note,
		     assigned_to = EXCLUDED.assigned_to,
		     last_fact_id = EXCLUDED.last_fact_id,
		     fact_count = EXCLUDED.fact_count,
		     updated_at_unix_nano = EXCLUDED.updated_at_unix_nano`,
		strings.TrimSpace(state.IssueID),
		strings.TrimSpace(state.CreatedBy),
		state.CreatedAt.UTC().UnixNano(),
		string(normalizeIssueStatus(state.Status)),
		closedAtUnixNano,
		closedBy,
		closedReason,
		closedReasonNote,
		strings.TrimSpace(state.AssignedTo),
		strings.TrimSpace(lastFactID),
		state.FactCount,
		time.Now().UTC().UnixNano(),
	); err != nil {
		return fmt.Errorf("upsert lifecycle projection for %q: %w", state.IssueID, err)
	}
	return nil
}

func mustLifecycleJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal lifecycle payload: %v", err))
	}
	return json.RawMessage(encoded)
}
