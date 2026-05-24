package appendonly

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"sortit/internal/issues"
)

const (
	defaultLifecycleCheckpoint = "issue-lifecycle-backfill"
	lifecycleDomain            = "issue-lifecycle"

	kindCreated    = "created"
	kindAssigned   = "assigned"
	kindClosed     = "closed"
	kindReopened   = "reopened"
	kindRefinement = "refinement"
)

type LifecycleFact struct {
	ID        string          `json:"id"`
	IssueID   string          `json:"issueId"`
	Sequence  int64           `json:"sequence"`
	Kind      string          `json:"kind"`
	CreatedBy string          `json:"createdBy"`
	CreatedAt time.Time       `json:"createdAt"`
	Payload   json.RawMessage `json:"payload"`
	Source    string          `json:"source"`
	SourceID  string          `json:"sourceId"`
	Inferred  bool            `json:"inferred"`
}

type LifecycleProjection struct {
	IssueID          string             `json:"issueId"`
	CreatedBy        string             `json:"createdBy"`
	CreatedAt        time.Time          `json:"createdAt"`
	Status           issues.IssueStatus `json:"status"`
	ClosedAt         *time.Time         `json:"closedAt,omitempty"`
	ClosedBy         string             `json:"closedBy,omitempty"`
	ClosedReason     string             `json:"closedReason,omitempty"`
	ClosedReasonNote string             `json:"closedReasonNote,omitempty"`
	AssignedTo       string             `json:"assignedTo,omitempty"`
	LastFactID       string             `json:"lastFactId,omitempty"`
	FactCount        int                `json:"factCount"`
	UpdatedAt        time.Time          `json:"updatedAt"`
}

type LifecycleBackfillResult struct {
	Checkpoint       string `json:"checkpoint"`
	ProcessedIssues  int    `json:"processedIssues"`
	WroteFacts       int    `json:"wroteFacts"`
	WroteProjections int    `json:"wroteProjections"`
	LastIssueID      string `json:"lastIssueId,omitempty"`
	Complete         bool   `json:"complete"`
}

type LifecycleParityMismatch struct {
	IssueID string   `json:"issueId"`
	Fields  []string `json:"fields"`
}

type LifecycleParityResult struct {
	Domain             string                    `json:"domain"`
	TotalIssues        int                       `json:"totalIssues"`
	ProjectionCount    int                       `json:"projectionCount"`
	MissingProjections int                       `json:"missingProjections"`
	MismatchedIssues   int                       `json:"mismatchedIssues"`
	Mismatches         []LifecycleParityMismatch `json:"mismatches,omitempty"`
}

type lifecycleSource interface {
	List(ctx context.Context) ([]issues.Issue, error)
	GetIssueDetail(ctx context.Context, id string) (issues.Issue, error)
}

type lifecycleCursor struct {
	LastIssueID string `json:"lastIssueId"`
}

type lifecycleAssignedPayload struct {
	AssignedTo string `json:"assignedTo"`
	Body       string `json:"body,omitempty"`
}

type lifecycleClosedPayload struct {
	ClosedAtUnixNano int64  `json:"closedAtUnixNano"`
	ClosedBy         string `json:"closedBy"`
	Reason           string `json:"reason,omitempty"`
	ReasonNote       string `json:"reasonNote,omitempty"`
	Body             string `json:"body,omitempty"`
}

type lifecycleRefinedPayload struct {
	Body string `json:"body"`
}

type lifecycleFallbackPayload struct {
	ObservedState string `json:"observedState,omitempty"`
}

type LifecycleStore struct {
	db *sql.DB
}

func NewLifecycleStore(db *sql.DB) *LifecycleStore {
	return &LifecycleStore{db: db}
}

func (s *LifecycleStore) BackfillBatch(
	ctx context.Context,
	source lifecycleSource,
	checkpointName string,
	batchSize int,
) (LifecycleBackfillResult, error) {
	checkpointName = strings.TrimSpace(checkpointName)
	if checkpointName == "" {
		checkpointName = defaultLifecycleCheckpoint
	}
	if batchSize <= 0 {
		batchSize = 100
	}

	framework := NewFrameworkStore(s.db)
	checkpoint, ok, err := framework.GetCheckpoint(ctx, checkpointName)
	if err != nil {
		return LifecycleBackfillResult{}, err
	}

	cursor := lifecycleCursor{}
	if ok && len(checkpoint.CursorJSON) > 0 {
		if err := json.Unmarshal(checkpoint.CursorJSON, &cursor); err != nil {
			return LifecycleBackfillResult{}, fmt.Errorf("decode lifecycle checkpoint cursor: %w", err)
		}
	}

	items, err := source.List(ctx)
	if err != nil {
		return LifecycleBackfillResult{}, fmt.Errorf("list issues for lifecycle backfill: %w", err)
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.TrimSpace(items[i].ID) < strings.TrimSpace(items[j].ID)
	})

	candidates := make([]issues.Issue, 0, batchSize)
	remainingCount := 0
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if cursor.LastIssueID != "" && id <= cursor.LastIssueID {
			continue
		}
		remainingCount++
		if len(candidates) < batchSize {
			candidates = append(candidates, item)
		}
	}

	result := LifecycleBackfillResult{
		Checkpoint: checkpointName,
		Complete:   len(candidates) == 0,
	}
	if len(candidates) == 0 {
		if err := framework.UpsertCheckpoint(ctx, Checkpoint{
			Name:        checkpointName,
			Phase:       "completed",
			CursorJSON:  mustMarshalJSON(cursor),
			SummaryJSON: mustMarshalJSON(result),
		}); err != nil {
			return LifecycleBackfillResult{}, err
		}
		return result, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return LifecycleBackfillResult{}, fmt.Errorf("begin lifecycle backfill tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, item := range candidates {
		detail, err := source.GetIssueDetail(ctx, item.ID)
		if err != nil {
			return LifecycleBackfillResult{}, fmt.Errorf("load issue detail %q for lifecycle backfill: %w", item.ID, err)
		}

		facts, projection, err := deriveLifecycleFacts(detail)
		if err != nil {
			return LifecycleBackfillResult{}, fmt.Errorf("derive lifecycle facts for %q: %w", item.ID, err)
		}
		if err := s.replaceFactsForIssue(ctx, tx, detail.ID, facts); err != nil {
			return LifecycleBackfillResult{}, err
		}
		if err := s.upsertProjection(ctx, tx, projection); err != nil {
			return LifecycleBackfillResult{}, err
		}

		result.ProcessedIssues++
		result.WroteFacts += len(facts)
		result.WroteProjections++
		result.LastIssueID = detail.ID
	}

	if err := tx.Commit(); err != nil {
		return LifecycleBackfillResult{}, fmt.Errorf("commit lifecycle backfill tx: %w", err)
	}

	result.Complete = remainingCount <= batchSize
	cursor.LastIssueID = result.LastIssueID
	phase := "backfilling"
	if result.Complete {
		phase = "completed"
	}
	if err := framework.UpsertCheckpoint(ctx, Checkpoint{
		Name:        checkpointName,
		Phase:       phase,
		CursorJSON:  mustMarshalJSON(cursor),
		SummaryJSON: mustMarshalJSON(result),
	}); err != nil {
		return LifecycleBackfillResult{}, err
	}

	return result, nil
}

func (s *LifecycleStore) CheckParity(
	ctx context.Context,
	source lifecycleSource,
	detailLimit int,
) (LifecycleParityResult, error) {
	if detailLimit <= 0 {
		detailLimit = 20
	}

	items, err := source.List(ctx)
	if err != nil {
		return LifecycleParityResult{}, fmt.Errorf("list issues for lifecycle parity: %w", err)
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.TrimSpace(items[i].ID) < strings.TrimSpace(items[j].ID)
	})

	projections, err := s.loadAllProjections(ctx)
	if err != nil {
		return LifecycleParityResult{}, err
	}

	result := LifecycleParityResult{
		Domain:          lifecycleDomain,
		TotalIssues:     len(items),
		ProjectionCount: len(projections),
	}

	for _, item := range items {
		projection, ok := projections[item.ID]
		if !ok {
			result.MissingProjections++
			result.MismatchedIssues++
			if len(result.Mismatches) < detailLimit {
				result.Mismatches = append(result.Mismatches, LifecycleParityMismatch{
					IssueID: item.ID,
					Fields:  []string{"missing_projection"},
				})
			}
			continue
		}

		fields := compareLifecycleProjection(item, projection)
		if len(fields) == 0 {
			continue
		}

		result.MismatchedIssues++
		if len(result.Mismatches) < detailLimit {
			result.Mismatches = append(result.Mismatches, LifecycleParityMismatch{
				IssueID: item.ID,
				Fields:  fields,
			})
		}
	}

	return result, nil
}

func (s *LifecycleStore) replaceFactsForIssue(
	ctx context.Context,
	tx *sql.Tx,
	issueID string,
	facts []LifecycleFact,
) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM issue_lifecycle_facts WHERE issue_id = $1`, issueID); err != nil {
		return fmt.Errorf("delete existing lifecycle facts for %q: %w", issueID, err)
	}

	for _, fact := range facts {
		payload, err := normalizeJSON(fact.Payload)
		if err != nil {
			return fmt.Errorf("normalize lifecycle fact payload for %q: %w", fact.ID, err)
		}
		if _, err := tx.ExecContext(
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
			fact.ID,
			fact.IssueID,
			fact.Sequence,
			fact.Kind,
			fact.CreatedBy,
			fact.CreatedAt.UTC().UnixNano(),
			[]byte(payload),
			fact.Source,
			fact.SourceID,
			fact.Inferred,
		); err != nil {
			return fmt.Errorf("insert lifecycle fact %q: %w", fact.ID, err)
		}
	}

	return nil
}

func (s *LifecycleStore) upsertProjection(
	ctx context.Context,
	tx *sql.Tx,
	projection LifecycleProjection,
) error {
	var closedAtUnixNano int64
	if projection.ClosedAt != nil {
		closedAtUnixNano = projection.ClosedAt.UTC().UnixNano()
	}

	updatedAt := projection.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	if _, err := tx.ExecContext(
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
		projection.IssueID,
		projection.CreatedBy,
		projection.CreatedAt.UTC().UnixNano(),
		string(projection.Status),
		closedAtUnixNano,
		projection.ClosedBy,
		projection.ClosedReason,
		projection.ClosedReasonNote,
		projection.AssignedTo,
		projection.LastFactID,
		projection.FactCount,
		updatedAt.UnixNano(),
	); err != nil {
		return fmt.Errorf("upsert lifecycle projection for %q: %w", projection.IssueID, err)
	}

	return nil
}

func (s *LifecycleStore) loadAllProjections(ctx context.Context) (map[string]LifecycleProjection, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT issue_id,
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
		 FROM issue_lifecycle_projections`,
	)
	if err != nil {
		return nil, fmt.Errorf("list lifecycle projections: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	projections := make(map[string]LifecycleProjection)
	for rows.Next() {
		var (
			projection        LifecycleProjection
			createdAtUnixNano int64
			closedAtUnixNano  int64
			status            string
			updatedAtUnixNano int64
		)
		if err := rows.Scan(
			&projection.IssueID,
			&projection.CreatedBy,
			&createdAtUnixNano,
			&status,
			&closedAtUnixNano,
			&projection.ClosedBy,
			&projection.ClosedReason,
			&projection.ClosedReasonNote,
			&projection.AssignedTo,
			&projection.LastFactID,
			&projection.FactCount,
			&updatedAtUnixNano,
		); err != nil {
			return nil, fmt.Errorf("scan lifecycle projection: %w", err)
		}

		projection.CreatedAt = time.Unix(0, createdAtUnixNano).UTC()
		projection.Status = issues.IssueStatus(strings.TrimSpace(status))
		if closedAtUnixNano > 0 {
			closedAt := time.Unix(0, closedAtUnixNano).UTC()
			projection.ClosedAt = &closedAt
		}
		projection.UpdatedAt = time.Unix(0, updatedAtUnixNano).UTC()
		projections[projection.IssueID] = projection
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lifecycle projections: %w", err)
	}

	return projections, nil
}

func deriveLifecycleFacts(issue issues.Issue) ([]LifecycleFact, LifecycleProjection, error) {
	posts := append([]issues.IssuePost(nil), issue.Discussion...)
	sort.Slice(posts, func(i, j int) bool {
		if posts[i].Sequence != posts[j].Sequence {
			return posts[i].Sequence < posts[j].Sequence
		}
		if !posts[i].CreatedAt.Equal(posts[j].CreatedAt) {
			return posts[i].CreatedAt.Before(posts[j].CreatedAt)
		}
		return posts[i].ID < posts[j].ID
	})

	facts := make([]LifecycleFact, 0, len(posts)+2)
	maxSequence := 0
	for _, post := range posts {
		if post.Sequence > maxSequence {
			maxSequence = post.Sequence
		}
		fact, ok, err := lifecycleFactFromPost(issue.ID, post)
		if err != nil {
			return nil, LifecycleProjection{}, err
		}
		if ok {
			if fact.Kind == kindCreated {
				fact.CreatedBy = strings.TrimSpace(issue.CreatedBy)
				fact.CreatedAt = issue.CreatedAt.UTC()
			}
			facts = append(facts, fact)
		}
	}

	projection := buildLifecycleProjection(issue.ID, facts)
	if projection.CreatedAt.IsZero() {
		projection.CreatedAt = issue.CreatedAt.UTC()
	}
	if strings.TrimSpace(projection.CreatedBy) == "" {
		projection.CreatedBy = strings.TrimSpace(issue.CreatedBy)
	}
	if projection.Status == "" {
		projection.Status = issues.StatusOpen
	}

	facts, projection = appendLifecycleFallbackFacts(issue, facts, projection, maxSequence)
	projection.FactCount = len(facts)
	if len(facts) > 0 {
		projection.LastFactID = facts[len(facts)-1].ID
	}
	projection.UpdatedAt = time.Now().UTC()

	return facts, projection, nil
}

func lifecycleFactFromPost(issueID string, post issues.IssuePost) (LifecycleFact, bool, error) {
	sourceID := strings.TrimSpace(post.ID)
	if sourceID == "" {
		return LifecycleFact{}, false, nil
	}

	base := LifecycleFact{
		ID:        "issue-lifecycle-fact:" + sourceID,
		IssueID:   issueID,
		Sequence:  int64(post.Sequence),
		CreatedBy: strings.TrimSpace(post.CreatedBy),
		CreatedAt: post.CreatedAt.UTC(),
		Source:    "issue_post",
		SourceID:  sourceID,
	}

	switch strings.TrimSpace(post.Kind) {
	case "":
		if post.Sequence != 1 {
			return LifecycleFact{}, false, nil
		}
		base.Kind = kindCreated
		base.Payload = mustMarshalJSON(map[string]any{
			"raw": strings.TrimSpace(post.Raw),
		})
		return base, true, nil
	case kindAssigned:
		assignedTo, err := parseAssignedTo(post.Raw)
		if err != nil {
			return LifecycleFact{}, false, err
		}
		base.Kind = kindAssigned
		base.Payload = mustMarshalJSON(lifecycleAssignedPayload{
			AssignedTo: assignedTo,
			Body:       strings.TrimSpace(post.Raw),
		})
		return base, true, nil
	case kindClosed:
		closedBy, reason, reasonNote := parseClosedPost(post.CreatedBy, post.Raw)
		base.Kind = kindClosed
		base.Payload = mustMarshalJSON(lifecycleClosedPayload{
			ClosedAtUnixNano: post.CreatedAt.UTC().UnixNano(),
			ClosedBy:         closedBy,
			Reason:           reason,
			ReasonNote:       reasonNote,
			Body:             strings.TrimSpace(post.Raw),
		})
		return base, true, nil
	case kindReopened:
		base.Kind = kindReopened
		base.Payload = mustMarshalJSON(lifecycleFallbackPayload{
			ObservedState: "open",
		})
		return base, true, nil
	case kindRefinement:
		base.Kind = "refined"
		base.Payload = mustMarshalJSON(lifecycleRefinedPayload{
			Body: strings.TrimSpace(post.Raw),
		})
		return base, true, nil
	default:
		return LifecycleFact{}, false, nil
	}
}

func buildLifecycleProjection(issueID string, facts []LifecycleFact) LifecycleProjection {
	ordered := append([]LifecycleFact(nil), facts...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Sequence != ordered[j].Sequence {
			return ordered[i].Sequence < ordered[j].Sequence
		}
		if !ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
		}
		return ordered[i].ID < ordered[j].ID
	})

	projection := LifecycleProjection{
		IssueID: issueID,
		Status:  issues.StatusOpen,
	}

	for _, fact := range ordered {
		projection.LastFactID = fact.ID
		switch fact.Kind {
		case kindCreated:
			projection.CreatedBy = strings.TrimSpace(fact.CreatedBy)
			projection.CreatedAt = fact.CreatedAt.UTC()
			projection.Status = issues.StatusOpen
		case kindAssigned:
			var payload lifecycleAssignedPayload
			if err := json.Unmarshal(fact.Payload, &payload); err == nil {
				projection.AssignedTo = strings.TrimSpace(payload.AssignedTo)
			}
		case kindClosed:
			var payload lifecycleClosedPayload
			if err := json.Unmarshal(fact.Payload, &payload); err == nil {
				projection.Status = issues.StatusClosed
				closedAt := fact.CreatedAt.UTC()
				if payload.ClosedAtUnixNano > 0 {
					closedAt = time.Unix(0, payload.ClosedAtUnixNano).UTC()
				}
				projection.ClosedAt = &closedAt
				projection.ClosedBy = strings.TrimSpace(payload.ClosedBy)
				if projection.ClosedBy == "" {
					projection.ClosedBy = strings.TrimSpace(fact.CreatedBy)
				}
				projection.ClosedReason = strings.TrimSpace(payload.Reason)
				projection.ClosedReasonNote = strings.TrimSpace(payload.ReasonNote)
			}
		case kindReopened:
			projection.Status = issues.StatusOpen
			projection.ClosedAt = nil
			projection.ClosedBy = ""
			projection.ClosedReason = ""
			projection.ClosedReasonNote = ""
		}
	}

	return projection
}

func appendLifecycleFallbackFacts(
	issue issues.Issue,
	facts []LifecycleFact,
	projection LifecycleProjection,
	maxSequence int,
) ([]LifecycleFact, LifecycleProjection) {
	legacyAssigned := strings.TrimSpace(issue.AssignedTo)
	if projection.AssignedTo != legacyAssigned {
		maxSequence++
		facts = append(facts, LifecycleFact{
			ID:        fmt.Sprintf("issue-lifecycle-fact:%s:inferred-assigned", issue.ID),
			IssueID:   issue.ID,
			Sequence:  int64(maxSequence),
			Kind:      kindAssigned,
			CreatedBy: "",
			CreatedAt: issue.CreatedAt.UTC(),
			Payload: mustMarshalJSON(lifecycleAssignedPayload{
				AssignedTo: legacyAssigned,
			}),
			Source:   "issue_legacy_state",
			SourceID: issue.ID + ":assigned",
			Inferred: true,
		})
		projection.AssignedTo = legacyAssigned
	}

	switch issue.Status {
	case issues.StatusClosed:
		if !lifecycleCloseMatches(issue, projection) {
			maxSequence++
			closedAt := issue.CreatedAt.UTC()
			if issue.ClosedAt != nil {
				closedAt = issue.ClosedAt.UTC()
			}
			closedBy := strings.TrimSpace(issue.ClosedBy)
			facts = append(facts, LifecycleFact{
				ID:        fmt.Sprintf("issue-lifecycle-fact:%s:inferred-closed", issue.ID),
				IssueID:   issue.ID,
				Sequence:  int64(maxSequence),
				Kind:      kindClosed,
				CreatedBy: closedBy,
				CreatedAt: closedAt,
				Payload: mustMarshalJSON(lifecycleClosedPayload{
					ClosedAtUnixNano: closedAt.UnixNano(),
					ClosedBy:         closedBy,
					Reason:           strings.TrimSpace(issue.ClosedReason),
					ReasonNote:       strings.TrimSpace(issue.ClosedReasonNote),
				}),
				Source:   "issue_legacy_state",
				SourceID: issue.ID + ":closed",
				Inferred: true,
			})
			projection.Status = issues.StatusClosed
			projection.ClosedAt = &closedAt
			projection.ClosedBy = closedBy
			projection.ClosedReason = strings.TrimSpace(issue.ClosedReason)
			projection.ClosedReasonNote = strings.TrimSpace(issue.ClosedReasonNote)
		}
	default:
		if projection.Status != issues.StatusOpen ||
			projection.ClosedAt != nil ||
			projection.ClosedBy != "" ||
			projection.ClosedReason != "" ||
			projection.ClosedReasonNote != "" {
			maxSequence++
			facts = append(facts, LifecycleFact{
				ID:        fmt.Sprintf("issue-lifecycle-fact:%s:inferred-reopened", issue.ID),
				IssueID:   issue.ID,
				Sequence:  int64(maxSequence),
				Kind:      kindReopened,
				CreatedBy: "",
				CreatedAt: issue.CreatedAt.UTC(),
				Payload: mustMarshalJSON(lifecycleFallbackPayload{
					ObservedState: "open",
				}),
				Source:   "issue_legacy_state",
				SourceID: issue.ID + ":reopened",
				Inferred: true,
			})
			projection.Status = issues.StatusOpen
			projection.ClosedAt = nil
			projection.ClosedBy = ""
			projection.ClosedReason = ""
			projection.ClosedReasonNote = ""
		}
	}

	return facts, projection
}

func lifecycleCloseMatches(issue issues.Issue, projection LifecycleProjection) bool {
	if projection.Status != issues.StatusClosed {
		return false
	}

	legacyClosedAt := int64(0)
	if issue.ClosedAt != nil {
		legacyClosedAt = issue.ClosedAt.UTC().UnixNano()
	}
	projectedClosedAt := int64(0)
	if projection.ClosedAt != nil {
		projectedClosedAt = projection.ClosedAt.UTC().UnixNano()
	}

	return legacyClosedAt == projectedClosedAt &&
		strings.TrimSpace(issue.ClosedBy) == strings.TrimSpace(projection.ClosedBy) &&
		strings.TrimSpace(issue.ClosedReason) == strings.TrimSpace(projection.ClosedReason) &&
		strings.TrimSpace(issue.ClosedReasonNote) == strings.TrimSpace(projection.ClosedReasonNote)
}

func compareLifecycleProjection(issue issues.Issue, projection LifecycleProjection) []string {
	fields := make([]string, 0, 8)

	if strings.TrimSpace(issue.CreatedBy) != strings.TrimSpace(projection.CreatedBy) {
		fields = append(fields, "created_by")
	}
	if issue.CreatedAt.UTC().UnixNano() != projection.CreatedAt.UTC().UnixNano() {
		fields = append(fields, "created_at")
	}
	if issue.Status != projection.Status {
		fields = append(fields, "status")
	}
	if strings.TrimSpace(issue.AssignedTo) != strings.TrimSpace(projection.AssignedTo) {
		fields = append(fields, "assigned_to")
	}

	legacyClosedAt := int64(0)
	if issue.ClosedAt != nil {
		legacyClosedAt = issue.ClosedAt.UTC().UnixNano()
	}
	projectedClosedAt := int64(0)
	if projection.ClosedAt != nil {
		projectedClosedAt = projection.ClosedAt.UTC().UnixNano()
	}
	if legacyClosedAt != projectedClosedAt {
		fields = append(fields, "closed_at")
	}
	if strings.TrimSpace(issue.ClosedBy) != strings.TrimSpace(projection.ClosedBy) {
		fields = append(fields, "closed_by")
	}
	if strings.TrimSpace(issue.ClosedReason) != strings.TrimSpace(projection.ClosedReason) {
		fields = append(fields, "closed_reason")
	}
	if strings.TrimSpace(issue.ClosedReasonNote) != strings.TrimSpace(projection.ClosedReasonNote) {
		fields = append(fields, "closed_reason_note")
	}

	return fields
}

func parseAssignedTo(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "Unassigned issue." {
		return "", nil
	}
	const prefix = "Assigned to "
	if strings.HasPrefix(trimmed, prefix) && strings.HasSuffix(trimmed, ".") {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, prefix), ".")), nil
	}
	return "", fmt.Errorf("unsupported assigned post format %q", raw)
}

func parseClosedPost(createdBy string, raw string) (closedBy string, reason string, reasonNote string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return strings.TrimSpace(createdBy), "", ""
	}

	if strings.HasPrefix(trimmed, "Closed by ") && strings.HasSuffix(trimmed, ".") {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "Closed by "), ".")), "", ""
	}

	const marker = " closed this as "
	if index := strings.Index(trimmed, marker); index >= 0 {
		closedBy = strings.TrimSpace(trimmed[:index])
		remainder := strings.TrimSpace(trimmed[index+len(marker):])
		if split := strings.SplitN(remainder, " — ", 2); len(split) == 2 {
			return closedBy, strings.TrimSpace(split[0]), strings.TrimSpace(split[1])
		}
		return closedBy, strings.TrimSpace(strings.TrimSuffix(remainder, ".")), ""
	}

	return strings.TrimSpace(createdBy), "", ""
}

func mustMarshalJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal append-only json payload: %v", err))
	}
	return json.RawMessage(encoded)
}
