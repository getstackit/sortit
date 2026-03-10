package issues

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"

	"splat/internal/issues/issuesdb"
)

const (
	pgDriverName          = "pgx"
	schemaMigrationsTable = "schema_migrations"
)

//go:embed pgmigrations/*.sql
var pgMigrationsFS embed.FS

type PostgresStore struct {
	db      *sql.DB
	queries *issuesdb.Queries
}

func OpenPostgresStore(ctx context.Context, connString string) (*PostgresStore, error) {
	connString = strings.TrimSpace(connString)
	if connString == "" {
		return nil, fmt.Errorf("postgres connection string is required")
	}

	db, err := sql.Open(pgDriverName, connString)
	if err != nil {
		return nil, fmt.Errorf("open postgres database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	store := &PostgresStore{
		db:      db,
		queries: issuesdb.New(db),
	}

	if err := store.init(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) DB() *sql.DB {
	return s.db
}

func (s *PostgresStore) List(ctx context.Context) ([]Issue, error) {
	rows, err := s.queries.ListIssues(ctx)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}

	items := make([]Issue, 0, len(rows))
	for _, row := range rows {
		issue, err := issueFromQuery(row)
		if err != nil {
			return nil, err
		}
		items = append(items, issue)
	}
	return cloneIssues(items), nil
}

func (s *PostgresStore) Get(ctx context.Context, id string) (Issue, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Issue{}, ErrNotFound
	}

	return s.getIssueWithDiscussion(ctx, s.queries, id)
}

func (s *PostgresStore) Create(ctx context.Context, input CreateInput) (Issue, error) {
	raw := strings.TrimSpace(input.Raw)
	if raw == "" {
		return Issue{}, fmt.Errorf("raw is required")
	}

	createdBy := strings.TrimSpace(input.CreatedBy)
	if createdBy == "" {
		createdBy = "You"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Issue{}, fmt.Errorf("begin create issue tx: %w", err)
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)
	seq, err := qtx.NextIssueSeq(ctx)
	if err != nil {
		return Issue{}, fmt.Errorf("next issue sequence: %w", err)
	}

	issue := Issue{
		ID:        fmt.Sprintf("issue-%06d", seq),
		Raw:       raw,
		Tags:      displayTags(input.Tags, input.TagScores),
		CreatedBy: createdBy,
		CreatedAt: time.Now().UTC(),
		Status:    StatusOpen,
		TagScores: copyTagScores(input.TagScores),
		Embedding: copyEmbedding(input.Embedding),
	}
	issue.Discussion = initialDiscussion(issue)

	record, err := recordFromIssue(issue)
	if err != nil {
		return Issue{}, err
	}

	if err := qtx.InsertIssue(ctx, issuesdb.InsertIssueParams{
		ID:                record.ID,
		Raw:               record.Raw,
		TagsJson:          record.TagsJSON,
		CreatedBy:         record.CreatedBy,
		CreatedAtUnixNano: record.CreatedAtUnixNano,
		Status:            record.Status,
		ClosedAtUnixNano:  record.ClosedAtUnixNano,
		ClosedBy:          record.ClosedBy,
		TagScoresJson:     record.TagScoresJSON,
		EmbeddingJson:     record.EmbeddingJSON,
		AssignedTo:        record.AssignedTo,
	}); err != nil {
		return Issue{}, fmt.Errorf("insert issue: %w", err)
	}
	if err := insertIssuePosts(ctx, qtx, issue.Discussion); err != nil {
		return Issue{}, err
	}

	if err := tx.Commit(); err != nil {
		return Issue{}, fmt.Errorf("commit create issue: %w", err)
	}

	return cloneIssues([]Issue{issue})[0], nil
}

func (s *PostgresStore) Refine(ctx context.Context, id string, input RefineInput) (Issue, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Issue{}, ErrNotFound
	}

	postRaw := strings.TrimSpace(input.PostRaw)
	if postRaw == "" {
		return Issue{}, fmt.Errorf("post raw is required")
	}

	canonicalRaw := strings.TrimSpace(input.CanonicalRaw)
	if canonicalRaw == "" {
		return Issue{}, fmt.Errorf("canonical raw is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Issue{}, fmt.Errorf("begin refine issue tx: %w", err)
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)
	current, err := s.getIssueWithDiscussion(ctx, qtx, id)
	if err != nil {
		return Issue{}, err
	}

	if current.Status == StatusClosed {
		return Issue{}, ErrIssueClosed
	}

	post := IssuePost{
		ID:        issuePostID(id, len(current.Discussion)+1),
		IssueID:   id,
		Raw:       postRaw,
		CreatedBy: defaultActor(input.CreatedBy),
		CreatedAt: time.Now().UTC(),
		Sequence:  len(current.Discussion) + 1,
		Kind:      "refinement",
	}

	updated := current
	updated.Raw = canonicalRaw
	updated.Tags = displayTags(input.Tags, input.TagScores)
	updated.TagScores = copyTagScores(input.TagScores)
	updated.Embedding = copyEmbedding(input.Embedding)
	updated.Discussion = append(cloneIssuePosts(current.Discussion), post)

	record, err := recordFromIssue(updated)
	if err != nil {
		return Issue{}, err
	}

	if err := qtx.InsertIssuePost(ctx, issuesdb.InsertIssuePostParams{
		ID:                post.ID,
		IssueID:           post.IssueID,
		Raw:               post.Raw,
		CreatedBy:         post.CreatedBy,
		CreatedAtUnixNano: post.CreatedAt.UnixNano(),
		Sequence:          int64(post.Sequence),
		Kind:              post.Kind,
	}); err != nil {
		return Issue{}, fmt.Errorf("insert issue post: %w", err)
	}
	if err := qtx.UpdateIssueRefinement(ctx, issuesdb.UpdateIssueRefinementParams{
		Raw:           record.Raw,
		TagsJson:      record.TagsJSON,
		TagScoresJson: record.TagScoresJSON,
		EmbeddingJson: record.EmbeddingJSON,
		ID:            updated.ID,
	}); err != nil {
		return Issue{}, fmt.Errorf("update issue refinement: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Issue{}, fmt.Errorf("commit refine issue tx: %w", err)
	}

	return cloneIssues([]Issue{updated})[0], nil
}

func (s *PostgresStore) ProgressPost(ctx context.Context, id string, input ProgressInput) (Issue, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Issue{}, ErrNotFound
	}

	raw := strings.TrimSpace(input.Raw)
	if raw == "" {
		return Issue{}, fmt.Errorf("raw is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Issue{}, fmt.Errorf("begin progress post tx: %w", err)
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)
	current, err := s.getIssueWithDiscussion(ctx, qtx, id)
	if err != nil {
		return Issue{}, err
	}

	if current.Status == StatusClosed {
		return Issue{}, ErrIssueClosed
	}

	post := IssuePost{
		ID:        issuePostID(id, len(current.Discussion)+1),
		IssueID:   id,
		Raw:       raw,
		CreatedBy: defaultActor(input.CreatedBy),
		CreatedAt: time.Now().UTC(),
		Sequence:  len(current.Discussion) + 1,
		Kind:      "progress",
	}

	if err := qtx.InsertIssuePost(ctx, issuesdb.InsertIssuePostParams{
		ID:                post.ID,
		IssueID:           post.IssueID,
		Raw:               post.Raw,
		CreatedBy:         post.CreatedBy,
		CreatedAtUnixNano: post.CreatedAt.UnixNano(),
		Sequence:          int64(post.Sequence),
		Kind:              post.Kind,
	}); err != nil {
		return Issue{}, fmt.Errorf("insert progress post: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Issue{}, fmt.Errorf("commit progress post tx: %w", err)
	}

	updated := current
	updated.Discussion = append(cloneIssuePosts(current.Discussion), post)
	return cloneIssues([]Issue{updated})[0], nil
}

func (s *PostgresStore) CloseIssue(ctx context.Context, id string, closedBy string) (Issue, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Issue{}, ErrNotFound
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Issue{}, fmt.Errorf("begin close issue tx: %w", err)
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)
	issue, err := s.getIssueWithDiscussion(ctx, qtx, id)
	if err != nil {
		return Issue{}, fmt.Errorf("load issue for close: %w", err)
	}
	if issue.Status == StatusClosed && issue.ClosedAt != nil {
		if err := tx.Commit(); err != nil {
			return Issue{}, fmt.Errorf("commit close issue tx: %w", err)
		}
		return issue, nil
	}

	closedAt := time.Now().UTC()
	if err := qtx.CloseIssue(ctx, issuesdb.CloseIssueParams{
		ID:               id,
		ClosedAtUnixNano: closedAt.UnixNano(),
		ClosedBy:         defaultActor(closedBy),
	}); err != nil {
		return Issue{}, fmt.Errorf("close issue: %w", err)
	}

	updated, err := s.getIssueWithDiscussion(ctx, qtx, id)
	if err != nil {
		return Issue{}, fmt.Errorf("load closed issue: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Issue{}, fmt.Errorf("commit close issue tx: %w", err)
	}
	return updated, nil
}

func (s *PostgresStore) ReopenIssue(ctx context.Context, id string) (Issue, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Issue{}, ErrNotFound
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Issue{}, fmt.Errorf("begin reopen issue tx: %w", err)
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)
	issue, err := s.getIssueWithDiscussion(ctx, qtx, id)
	if err != nil {
		return Issue{}, fmt.Errorf("load issue for reopen: %w", err)
	}
	if issue.Status == StatusOpen {
		if err := tx.Commit(); err != nil {
			return Issue{}, fmt.Errorf("commit reopen issue tx: %w", err)
		}
		return issue, nil
	}

	if err := qtx.ReopenIssue(ctx, id); err != nil {
		return Issue{}, fmt.Errorf("reopen issue: %w", err)
	}

	updated, err := s.getIssueWithDiscussion(ctx, qtx, id)
	if err != nil {
		return Issue{}, fmt.Errorf("load reopened issue: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Issue{}, fmt.Errorf("commit reopen issue tx: %w", err)
	}
	return updated, nil
}

func (s *PostgresStore) AssignIssue(ctx context.Context, id, assignee string) (Issue, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Issue{}, ErrNotFound
	}

	assignee = strings.TrimSpace(assignee)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Issue{}, fmt.Errorf("begin assign issue tx: %w", err)
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)

	if err := qtx.AssignIssue(ctx, issuesdb.AssignIssueParams{
		AssignedTo: assignee,
		ID:         id,
	}); err != nil {
		return Issue{}, fmt.Errorf("assign issue: %w", err)
	}

	issue, err := s.getIssueWithDiscussion(ctx, qtx, id)
	if err != nil {
		return Issue{}, err
	}

	if err := tx.Commit(); err != nil {
		return Issue{}, fmt.Errorf("commit assign issue tx: %w", err)
	}

	return issue, nil
}

func (s *PostgresStore) SplitIssue(ctx context.Context, input SplitInput) (IssueOperationResult, error) {
	sourceID := strings.TrimSpace(input.SourceID)
	if sourceID == "" {
		return IssueOperationResult{}, ErrNotFound
	}
	if len(input.Children) == 0 {
		return IssueOperationResult{}, fmt.Errorf("at least one child issue is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IssueOperationResult{}, fmt.Errorf("begin split issue tx: %w", err)
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)
	source, err := s.getIssueWithDiscussion(ctx, qtx, sourceID)
	if err != nil {
		return IssueOperationResult{}, err
	}

	actor := defaultActor(input.CreatedBy)
	createdAt := time.Now().UTC()
	operation, err := nextIssueOperation(ctx, qtx, IssueOperation{
		Kind:      IssueOperationKindSplit,
		CreatedBy: actor,
		CreatedAt: createdAt,
		Note:      strings.TrimSpace(input.Note),
	})
	if err != nil {
		return IssueOperationResult{}, err
	}
	operation.Participants = append(operation.Participants, IssueOperationParticipant{
		IssueID: sourceID,
		Role:    "source",
	})

	if err := insertIssueOperation(ctx, qtx, operation); err != nil {
		return IssueOperationResult{}, err
	}
	if err := insertIssueOperationParticipant(ctx, qtx, operation.ID, IssueOperationParticipant{
		IssueID: sourceID,
		Role:    "source",
	}, 1); err != nil {
		return IssueOperationResult{}, err
	}

	createdIssues := make([]Issue, 0, len(input.Children))
	touched := []IssueReference{issueReference(source)}
	for index, child := range input.Children {
		raw := strings.TrimSpace(child.Raw)
		if raw == "" {
			return IssueOperationResult{}, fmt.Errorf("child raw is required")
		}
		issueSeq, err := qtx.NextIssueSeq(ctx)
		if err != nil {
			return IssueOperationResult{}, fmt.Errorf("next issue sequence: %w", err)
		}
		issue := Issue{
			ID:        fmt.Sprintf("issue-%06d", issueSeq),
			Raw:       raw,
			Tags:      displayTags(child.Tags, child.TagScores),
			CreatedBy: actor,
			CreatedAt: createdAt,
			Status:    StatusOpen,
			TagScores: copyTagScores(child.TagScores),
			Embedding: copyEmbedding(child.Embedding),
		}
		issue.Discussion = initialDiscussion(issue)
		record, err := recordFromIssue(issue)
		if err != nil {
			return IssueOperationResult{}, err
		}
		if err := qtx.InsertIssue(ctx, issuesdb.InsertIssueParams{
			ID:                record.ID,
			Raw:               record.Raw,
			TagsJson:          record.TagsJSON,
			CreatedBy:         record.CreatedBy,
			CreatedAtUnixNano: record.CreatedAtUnixNano,
			Status:            record.Status,
			ClosedAtUnixNano:  record.ClosedAtUnixNano,
			ClosedBy:          record.ClosedBy,
			TagScoresJson:     record.TagScoresJSON,
			EmbeddingJson:     record.EmbeddingJSON,
			AssignedTo:        record.AssignedTo,
		}); err != nil {
			return IssueOperationResult{}, fmt.Errorf("insert split child issue: %w", err)
		}
		if err := insertIssuePosts(ctx, qtx, issue.Discussion); err != nil {
			return IssueOperationResult{}, err
		}
		if err := insertIssueOperationParticipant(ctx, qtx, operation.ID, IssueOperationParticipant{
			IssueID: issue.ID,
			Role:    fmt.Sprintf("child:%d", index+1),
		}, index+2); err != nil {
			return IssueOperationResult{}, err
		}
		parentLink := IssueLink{
			ID:            fmt.Sprintf("%s-link-%06d", operation.ID, index*2+1),
			Type:          IssueLinkTypeParentOf,
			SourceIssueID: sourceID,
			TargetIssueID: issue.ID,
			CreatedBy:     actor,
			CreatedAt:     createdAt,
			Note:          operation.Note,
			OperationID:   operation.ID,
		}
		childLink := IssueLink{
			ID:            fmt.Sprintf("%s-link-%06d", operation.ID, index*2+2),
			Type:          IssueLinkTypeChildOf,
			SourceIssueID: issue.ID,
			TargetIssueID: sourceID,
			CreatedBy:     actor,
			CreatedAt:     createdAt,
			Note:          operation.Note,
			OperationID:   operation.ID,
		}
		if err := insertIssueLink(ctx, qtx, parentLink); err != nil {
			return IssueOperationResult{}, err
		}
		if err := insertIssueLink(ctx, qtx, childLink); err != nil {
			return IssueOperationResult{}, err
		}
		createdIssues = append(createdIssues, cloneIssues([]Issue{issue})[0])
		touched = append(touched, issueReference(issue))
	}

	if input.CloseSource {
		if err := qtx.CloseIssue(ctx, issuesdb.CloseIssueParams{
			ID:               sourceID,
			ClosedAtUnixNano: createdAt.UnixNano(),
			ClosedBy:         actor,
		}); err != nil {
			return IssueOperationResult{}, fmt.Errorf("close split source issue: %w", err)
		}
		source.Status = StatusClosed
		source.ClosedAt = &createdAt
		source.ClosedBy = actor
		touched[0] = issueReference(source)
	}

	result, err := hydrateIssueOperationResult(ctx, qtx, operation.ID, createdIssues, touched)
	if err != nil {
		return IssueOperationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return IssueOperationResult{}, fmt.Errorf("commit split issue tx: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) CombineIssues(ctx context.Context, input CombineInput) (IssueOperationResult, error) {
	sourceIDs := sanitizeIssueIDs(input.SourceIDs)
	if len(sourceIDs) < 2 {
		return IssueOperationResult{}, fmt.Errorf("at least two source issues are required")
	}
	raw := strings.TrimSpace(input.Raw)
	if raw == "" {
		return IssueOperationResult{}, fmt.Errorf("raw is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IssueOperationResult{}, fmt.Errorf("begin combine issues tx: %w", err)
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)
	actor := defaultActor(input.CreatedBy)
	createdAt := time.Now().UTC()

	touched := make([]IssueReference, 0, len(sourceIDs)+1)
	for _, id := range sourceIDs {
		source, err := s.getIssueWithDiscussion(ctx, qtx, id)
		if err != nil {
			return IssueOperationResult{}, err
		}
		touched = append(touched, issueReference(source))
	}

	issueSeq, err := qtx.NextIssueSeq(ctx)
	if err != nil {
		return IssueOperationResult{}, fmt.Errorf("next issue sequence: %w", err)
	}
	createdIssue := Issue{
		ID:        fmt.Sprintf("issue-%06d", issueSeq),
		Raw:       raw,
		Tags:      displayTags(input.Tags, input.TagScores),
		CreatedBy: actor,
		CreatedAt: createdAt,
		Status:    StatusOpen,
		TagScores: copyTagScores(input.TagScores),
		Embedding: copyEmbedding(input.Embedding),
	}
	createdIssue.Discussion = initialDiscussion(createdIssue)
	record, err := recordFromIssue(createdIssue)
	if err != nil {
		return IssueOperationResult{}, err
	}
	if err := qtx.InsertIssue(ctx, issuesdb.InsertIssueParams{
		ID:                record.ID,
		Raw:               record.Raw,
		TagsJson:          record.TagsJSON,
		CreatedBy:         record.CreatedBy,
		CreatedAtUnixNano: record.CreatedAtUnixNano,
		Status:            record.Status,
		ClosedAtUnixNano:  record.ClosedAtUnixNano,
		ClosedBy:          record.ClosedBy,
		TagScoresJson:     record.TagScoresJSON,
		EmbeddingJson:     record.EmbeddingJSON,
		AssignedTo:        record.AssignedTo,
	}); err != nil {
		return IssueOperationResult{}, fmt.Errorf("insert combined issue: %w", err)
	}
	if err := insertIssuePosts(ctx, qtx, createdIssue.Discussion); err != nil {
		return IssueOperationResult{}, err
	}

	operation, err := nextIssueOperation(ctx, qtx, IssueOperation{
		Kind:      IssueOperationKindCombine,
		CreatedBy: actor,
		CreatedAt: createdAt,
		Note:      strings.TrimSpace(input.Note),
	})
	if err != nil {
		return IssueOperationResult{}, err
	}
	if err := insertIssueOperation(ctx, qtx, operation); err != nil {
		return IssueOperationResult{}, err
	}
	if err := insertIssueOperationParticipant(ctx, qtx, operation.ID, IssueOperationParticipant{
		IssueID: createdIssue.ID,
		Role:    "result",
	}, 1); err != nil {
		return IssueOperationResult{}, err
	}

	for index, id := range sourceIDs {
		if err := qtx.CloseIssue(ctx, issuesdb.CloseIssueParams{
			ID:               id,
			ClosedAtUnixNano: createdAt.UnixNano(),
			ClosedBy:         actor,
		}); err != nil {
			return IssueOperationResult{}, fmt.Errorf("close merged source issue: %w", err)
		}
		if err := insertIssueOperationParticipant(ctx, qtx, operation.ID, IssueOperationParticipant{
			IssueID: id,
			Role:    fmt.Sprintf("source:%d", index+1),
		}, index+2); err != nil {
			return IssueOperationResult{}, err
		}
		if err := insertIssueLink(ctx, qtx, IssueLink{
			ID:            fmt.Sprintf("%s-link-%06d", operation.ID, index+1),
			Type:          IssueLinkTypeMergedInto,
			SourceIssueID: id,
			TargetIssueID: createdIssue.ID,
			CreatedBy:     actor,
			CreatedAt:     createdAt,
			Note:          operation.Note,
			OperationID:   operation.ID,
		}); err != nil {
			return IssueOperationResult{}, err
		}
	}

	touched = append([]IssueReference{issueReference(createdIssue)}, touched...)
	for index := 1; index < len(touched); index++ {
		touched[index].Status = StatusClosed
	}

	result, err := hydrateIssueOperationResult(ctx, qtx, operation.ID, []Issue{cloneIssues([]Issue{createdIssue})[0]}, touched)
	if err != nil {
		return IssueOperationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return IssueOperationResult{}, fmt.Errorf("commit combine issues tx: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) LinkIssues(ctx context.Context, input LinkInput) (IssueOperationResult, error) {
	sourceID := strings.TrimSpace(input.SourceID)
	targetID := strings.TrimSpace(input.TargetID)
	if sourceID == "" || targetID == "" {
		return IssueOperationResult{}, ErrNotFound
	}
	linkType := normalizeIssueLinkType(input.Type)
	if linkType == "" {
		return IssueOperationResult{}, fmt.Errorf("link type is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IssueOperationResult{}, fmt.Errorf("begin link issues tx: %w", err)
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)
	source, err := s.getIssueWithDiscussion(ctx, qtx, sourceID)
	if err != nil {
		return IssueOperationResult{}, err
	}
	target, err := s.getIssueWithDiscussion(ctx, qtx, targetID)
	if err != nil {
		return IssueOperationResult{}, err
	}

	actor := defaultActor(input.CreatedBy)
	createdAt := time.Now().UTC()
	operation, err := nextIssueOperation(ctx, qtx, IssueOperation{
		Kind:      IssueOperationKindLink,
		CreatedBy: actor,
		CreatedAt: createdAt,
		Note:      strings.TrimSpace(input.Note),
	})
	if err != nil {
		return IssueOperationResult{}, err
	}
	if err := insertIssueOperation(ctx, qtx, operation); err != nil {
		return IssueOperationResult{}, err
	}
	if err := insertIssueOperationParticipant(ctx, qtx, operation.ID, IssueOperationParticipant{
		IssueID: sourceID,
		Role:    "source",
	}, 1); err != nil {
		return IssueOperationResult{}, err
	}
	if err := insertIssueOperationParticipant(ctx, qtx, operation.ID, IssueOperationParticipant{
		IssueID: targetID,
		Role:    "target",
	}, 2); err != nil {
		return IssueOperationResult{}, err
	}
	if err := insertIssueLink(ctx, qtx, IssueLink{
		ID:            fmt.Sprintf("%s-link-000001", operation.ID),
		Type:          linkType,
		SourceIssueID: sourceID,
		TargetIssueID: targetID,
		CreatedBy:     actor,
		CreatedAt:     createdAt,
		Note:          operation.Note,
		OperationID:   operation.ID,
	}); err != nil {
		return IssueOperationResult{}, err
	}

	result, err := hydrateIssueOperationResult(ctx, qtx, operation.ID, nil, []IssueReference{
		issueReference(source),
		issueReference(target),
	})
	if err != nil {
		return IssueOperationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return IssueOperationResult{}, fmt.Errorf("commit link issues tx: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) Replace(ctx context.Context, next []Issue) error {
	items := cloneIssues(next)
	slices.SortStableFunc(items, compareIssueOrder)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace issues tx: %w", err)
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)
	if err := qtx.DeleteAllIssueLinks(ctx); err != nil {
		return fmt.Errorf("clear issue links: %w", err)
	}
	if err := qtx.DeleteAllIssueOperationParticipants(ctx); err != nil {
		return fmt.Errorf("clear issue operation participants: %w", err)
	}
	if err := qtx.DeleteAllIssueOperations(ctx); err != nil {
		return fmt.Errorf("clear issue operations: %w", err)
	}
	if err := qtx.DeleteAllIssuePosts(ctx); err != nil {
		return fmt.Errorf("clear issue posts: %w", err)
	}
	if err := qtx.DeleteAllIssues(ctx); err != nil {
		return fmt.Errorf("clear issues: %w", err)
	}

	for _, issue := range items {
		record, err := recordFromIssue(issue)
		if err != nil {
			return err
		}

		if err := qtx.InsertIssue(ctx, issuesdb.InsertIssueParams{
			ID:                record.ID,
			Raw:               record.Raw,
			TagsJson:          record.TagsJSON,
			CreatedBy:         record.CreatedBy,
			CreatedAtUnixNano: record.CreatedAtUnixNano,
			Status:            record.Status,
			ClosedAtUnixNano:  record.ClosedAtUnixNano,
			ClosedBy:          record.ClosedBy,
			TagScoresJson:     record.TagScoresJSON,
			EmbeddingJson:     record.EmbeddingJSON,
			AssignedTo:        record.AssignedTo,
		}); err != nil {
			return fmt.Errorf("replace issue %q: %w", issue.ID, err)
		}

		if err := insertIssuePosts(ctx, qtx, initialDiscussion(issue)); err != nil {
			return err
		}
	}

	seqBase := int64(nextSequenceBase(items))
	if seqBase > 0 {
		if err := qtx.SetIssueSeq(ctx, seqBase); err != nil {
			return fmt.Errorf("set issue sequence: %w", err)
		}
	} else {
		if err := qtx.ResetIssueSeq(ctx); err != nil {
			return fmt.Errorf("reset issue sequence: %w", err)
		}
	}
	if err := qtx.ResetIssueOperationSeq(ctx); err != nil {
		return fmt.Errorf("reset issue operation sequence: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace issues: %w", err)
	}

	return nil
}

func (s *PostgresStore) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := s.queries.ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}

	tags := make([]Tag, 0, len(rows))
	for _, row := range rows {
		tag, err := tagFromQuery(row)
		if err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return cloneTags(tags), nil
}

func (s *PostgresStore) UpsertTags(ctx context.Context, tags []Tag) error {
	normalized := normalizeCatalogTags(tags)
	if len(normalized) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert tags tx: %w", err)
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)
	for _, tag := range normalized {
		record, err := recordFromTag(tag)
		if err != nil {
			return err
		}

		if err := qtx.UpsertTag(ctx, issuesdb.UpsertTagParams{
			Name:              record.Name,
			Description:       record.Description,
			CreatedAtUnixNano: record.CreatedAtUnixNano,
			EmbeddingJson:     record.EmbeddingJSON,
		}); err != nil {
			return fmt.Errorf("upsert tag %q: %w", tag.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert tags: %w", err)
	}

	return nil
}

func (s *PostgresStore) init(ctx context.Context) error {
	return s.runMigrations(ctx)
}

func (s *PostgresStore) runMigrations(ctx context.Context) error {
	_ = ctx
	dbDriver, err := migratepostgres.WithInstance(s.db, &migratepostgres.Config{
		MigrationsTable: schemaMigrationsTable,
	})
	if err != nil {
		return fmt.Errorf("create postgres migrate driver: %w", err)
	}

	sourceDriver, err := iofs.New(pgMigrationsFS, "pgmigrations")
	if err != nil {
		return fmt.Errorf("open postgres migrations: %w", err)
	}

	migrator, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", dbDriver)
	if err != nil {
		return fmt.Errorf("construct postgres migrator: %w", err)
	}

	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run postgres migrations: %w", err)
	}
	return nil
}

type issueRecord struct {
	ID                string
	Raw               string
	TagsJSON          json.RawMessage
	CreatedBy         string
	CreatedAtUnixNano int64
	Status            string
	ClosedAtUnixNano  int64
	ClosedBy          string
	TagScoresJSON     json.RawMessage
	EmbeddingJSON     json.RawMessage
	AssignedTo        string
}

type tagRecord struct {
	Name              string
	Description       string
	CreatedAtUnixNano int64
	EmbeddingJSON     json.RawMessage
}

type issueQuerier interface {
	GetIssue(context.Context, string) (issuesdb.Issue, error)
	ListIssues(context.Context) ([]issuesdb.Issue, error)
	ListIssuePosts(context.Context, string) ([]issuesdb.IssuePost, error)
	ListIssueLinksForIssue(context.Context, issuesdb.ListIssueLinksForIssueParams) ([]issuesdb.IssueLink, error)
	ListIssueOperationsForIssue(context.Context, string) ([]issuesdb.IssueOperation, error)
	ListIssueOperationParticipants(context.Context, string) ([]issuesdb.IssueOperationParticipant, error)
}

func recordFromIssue(issue Issue) (issueRecord, error) {
	tagsJSON, err := marshalJSONB(issue.Tags, []string{})
	if err != nil {
		return issueRecord{}, fmt.Errorf("marshal issue tags: %w", err)
	}
	tagScoresJSON, err := marshalJSONB(issue.TagScores, []TagRelevance{})
	if err != nil {
		return issueRecord{}, fmt.Errorf("marshal issue tag scores: %w", err)
	}
	embeddingJSON, err := marshalJSONB(issue.Embedding, []float64{})
	if err != nil {
		return issueRecord{}, fmt.Errorf("marshal issue embedding: %w", err)
	}

	return issueRecord{
		ID:                strings.TrimSpace(issue.ID),
		Raw:               strings.TrimSpace(issue.Raw),
		TagsJSON:          tagsJSON,
		CreatedBy:         strings.TrimSpace(issue.CreatedBy),
		CreatedAtUnixNano: issue.CreatedAt.UTC().UnixNano(),
		Status:            string(normalizeIssueStatus(issue.Status)),
		ClosedAtUnixNano:  closedAtUnixNano(issue),
		ClosedBy:          closedBy(issue),
		TagScoresJSON:     tagScoresJSON,
		EmbeddingJSON:     embeddingJSON,
		AssignedTo:        strings.TrimSpace(issue.AssignedTo),
	}, nil
}

func issueFromQuery(row issuesdb.Issue) (Issue, error) {
	tags, err := unmarshalJSONB[[]string](row.TagsJson)
	if err != nil {
		return Issue{}, fmt.Errorf("decode tags for %q: %w", row.ID, err)
	}
	tagScores, err := unmarshalJSONB[[]TagRelevance](row.TagScoresJson)
	if err != nil {
		return Issue{}, fmt.Errorf("decode tag scores for %q: %w", row.ID, err)
	}
	embedding, err := unmarshalJSONB[[]float64](row.EmbeddingJson)
	if err != nil {
		return Issue{}, fmt.Errorf("decode embedding for %q: %w", row.ID, err)
	}

	status := normalizeIssueStatus(IssueStatus(row.Status))
	closedAt := closedAtFromUnixNano(row.ClosedAtUnixNano)
	closedBy := strings.TrimSpace(row.ClosedBy)
	if status != StatusClosed {
		closedAt = nil
		closedBy = ""
	}

	return Issue{
		ID:         row.ID,
		Raw:        row.Raw,
		Tags:       tags,
		CreatedBy:  row.CreatedBy,
		CreatedAt:  time.Unix(0, row.CreatedAtUnixNano).UTC(),
		Status:     status,
		ClosedAt:   closedAt,
		ClosedBy:   closedBy,
		AssignedTo: strings.TrimSpace(row.AssignedTo),
		TagScores:  tagScores,
		Embedding:  embedding,
	}, nil
}

func issuePostFromQuery(row issuesdb.IssuePost) IssuePost {
	return IssuePost{
		ID:        row.ID,
		IssueID:   row.IssueID,
		Raw:       row.Raw,
		CreatedBy: row.CreatedBy,
		CreatedAt: time.Unix(0, row.CreatedAtUnixNano).UTC(),
		Sequence:  int(row.Sequence),
		Kind:      row.Kind,
	}
}

func issueLinkFromQuery(row issuesdb.IssueLink) IssueLink {
	return IssueLink{
		ID:            row.ID,
		Type:          normalizeIssueLinkType(IssueLinkType(row.Type)),
		SourceIssueID: row.SourceIssueID,
		TargetIssueID: row.TargetIssueID,
		CreatedBy:     row.CreatedBy,
		CreatedAt:     time.Unix(0, row.CreatedAtUnixNano).UTC(),
		Note:          strings.TrimSpace(row.Note),
		OperationID:   strings.TrimSpace(row.OperationID),
	}
}

func issueOperationFromQuery(row issuesdb.IssueOperation) IssueOperation {
	return IssueOperation{
		ID:        row.ID,
		Kind:      IssueOperationKind(strings.TrimSpace(row.Kind)),
		CreatedBy: row.CreatedBy,
		CreatedAt: time.Unix(0, row.CreatedAtUnixNano).UTC(),
		Note:      strings.TrimSpace(row.Note),
	}
}

func issueOperationParticipantFromQuery(row issuesdb.IssueOperationParticipant) IssueOperationParticipant {
	return IssueOperationParticipant{
		IssueID: row.IssueID,
		Role:    row.Role,
	}
}

func recordFromTag(tag Tag) (tagRecord, error) {
	embeddingJSON, err := marshalJSONB(tag.Embedding, []float64{})
	if err != nil {
		return tagRecord{}, fmt.Errorf("marshal tag embedding: %w", err)
	}

	createdAt := tag.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	return tagRecord{
		Name:              sanitizeTagName(tag.Name),
		Description:       strings.TrimSpace(tag.Description),
		CreatedAtUnixNano: createdAt.UnixNano(),
		EmbeddingJSON:     embeddingJSON,
	}, nil
}

func tagFromQuery(row issuesdb.Tag) (Tag, error) {
	embedding, err := unmarshalJSONB[[]float64](row.EmbeddingJson)
	if err != nil {
		return Tag{}, fmt.Errorf("decode embedding for tag %q: %w", row.Name, err)
	}

	return Tag{
		Name:        row.Name,
		Description: row.Description,
		CreatedAt:   time.Unix(0, row.CreatedAtUnixNano).UTC(),
		Embedding:   embedding,
	}, nil
}

func (s *PostgresStore) getIssueWithDiscussion(ctx context.Context, q issueQuerier, id string) (Issue, error) {
	issueRow, err := q.GetIssue(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Issue{}, ErrNotFound
		}
		return Issue{}, err
	}

	postRows, err := q.ListIssuePosts(ctx, id)
	if err != nil {
		return Issue{}, fmt.Errorf("list issue posts: %w", err)
	}
	linkRows, err := q.ListIssueLinksForIssue(ctx, issuesdb.ListIssueLinksForIssueParams{
		SourceIssueID: id,
		TargetIssueID: id,
	})
	if err != nil {
		return Issue{}, fmt.Errorf("list issue links: %w", err)
	}
	operationRows, err := q.ListIssueOperationsForIssue(ctx, id)
	if err != nil {
		return Issue{}, fmt.Errorf("list issue operations: %w", err)
	}
	allIssueRows, err := q.ListIssues(ctx)
	if err != nil {
		return Issue{}, fmt.Errorf("list issues for references: %w", err)
	}

	discussion := make([]IssuePost, 0, len(postRows))
	for _, row := range postRows {
		discussion = append(discussion, issuePostFromQuery(row))
	}
	links := make([]IssueLink, 0, len(linkRows))
	for _, row := range linkRows {
		links = append(links, issueLinkFromQuery(row))
	}

	issuesByID := make(map[string]Issue, len(allIssueRows))
	for _, row := range allIssueRows {
		issue, err := issueFromQuery(row)
		if err != nil {
			return Issue{}, err
		}
		issuesByID[issue.ID] = issue
	}

	operations := make([]IssueOperation, 0, len(operationRows))
	for _, row := range operationRows {
		participantsRows, err := q.ListIssueOperationParticipants(ctx, row.ID)
		if err != nil {
			return Issue{}, fmt.Errorf("list issue operation participants: %w", err)
		}
		operation := issueOperationFromQuery(row)
		participants := make([]IssueOperationParticipant, 0, len(participantsRows))
		for _, participantRow := range participantsRows {
			participant := issueOperationParticipantFromQuery(participantRow)
			if related, ok := issuesByID[participant.IssueID]; ok {
				ref := issueReference(related)
				participant.Issue = &ref
			}
			participants = append(participants, participant)
		}
		operation.Participants = participants
		operations = append(operations, operation)
	}

	return hydrateIssueWithDiscussion(issueRow, discussion, links, operations, issuesByID)
}

func hydrateIssueWithDiscussion(
	issueRow issuesdb.Issue,
	discussion []IssuePost,
	links []IssueLink,
	operations []IssueOperation,
	issuesByID map[string]Issue,
) (Issue, error) {
	issue, err := issueFromQuery(issueRow)
	if err != nil {
		return Issue{}, err
	}

	if len(discussion) == 0 {
		discussion = initialDiscussion(issue)
	}
	issue.Discussion = cloneIssuePosts(discussion)
	for _, link := range links {
		hydrated := link
		if hydrated.SourceIssueID == issue.ID {
			hydrated.Direction = "outgoing"
			if related, ok := issuesByID[hydrated.TargetIssueID]; ok {
				ref := issueReference(related)
				hydrated.RelatedIssue = &ref
			}
		} else {
			hydrated.Direction = "incoming"
			if related, ok := issuesByID[hydrated.SourceIssueID]; ok {
				ref := issueReference(related)
				hydrated.RelatedIssue = &ref
			}
		}
		issue.Links = append(issue.Links, hydrated)
	}
	issue.Operations = cloneIssueOperations(operations)
	return cloneIssues([]Issue{issue})[0], nil
}

func insertIssuePosts(ctx context.Context, q *issuesdb.Queries, posts []IssuePost) error {
	for _, post := range posts {
		if err := q.InsertIssuePost(ctx, issuesdb.InsertIssuePostParams{
			ID:                strings.TrimSpace(post.ID),
			IssueID:           strings.TrimSpace(post.IssueID),
			Raw:               strings.TrimSpace(post.Raw),
			CreatedBy:         strings.TrimSpace(post.CreatedBy),
			CreatedAtUnixNano: post.CreatedAt.UTC().UnixNano(),
			Sequence:          int64(post.Sequence),
			Kind:              post.Kind,
		}); err != nil {
			return fmt.Errorf("insert issue post %q: %w", post.ID, err)
		}
	}
	return nil
}

func closedAtUnixNano(issue Issue) int64 {
	if normalizeIssueStatus(issue.Status) != StatusClosed || issue.ClosedAt == nil {
		return 0
	}
	return issue.ClosedAt.UTC().UnixNano()
}

func closedBy(issue Issue) string {
	if normalizeIssueStatus(issue.Status) != StatusClosed {
		return ""
	}
	return strings.TrimSpace(issue.ClosedBy)
}

func closedAtFromUnixNano(value int64) *time.Time {
	if value <= 0 {
		return nil
	}

	closedAt := time.Unix(0, value).UTC()
	return &closedAt
}

func nextIssueOperation(ctx context.Context, q *issuesdb.Queries, operation IssueOperation) (IssueOperation, error) {
	seq, err := q.NextIssueOperationSeq(ctx)
	if err != nil {
		return IssueOperation{}, fmt.Errorf("next issue operation sequence: %w", err)
	}
	operation.ID = fmt.Sprintf("issue-op-%06d", seq)
	operation.Kind = IssueOperationKind(strings.TrimSpace(string(operation.Kind)))
	return operation, nil
}

func insertIssueOperation(ctx context.Context, q *issuesdb.Queries, operation IssueOperation) error {
	if err := q.InsertIssueOperation(ctx, issuesdb.InsertIssueOperationParams{
		ID:                strings.TrimSpace(operation.ID),
		Kind:              strings.TrimSpace(string(operation.Kind)),
		CreatedBy:         strings.TrimSpace(operation.CreatedBy),
		CreatedAtUnixNano: operation.CreatedAt.UTC().UnixNano(),
		Note:              strings.TrimSpace(operation.Note),
	}); err != nil {
		return fmt.Errorf("insert issue operation %q: %w", operation.ID, err)
	}
	return nil
}

func insertIssueOperationParticipant(
	ctx context.Context,
	q *issuesdb.Queries,
	operationID string,
	participant IssueOperationParticipant,
	sequence int,
) error {
	if err := q.InsertIssueOperationParticipant(ctx, issuesdb.InsertIssueOperationParticipantParams{
		OperationID: strings.TrimSpace(operationID),
		IssueID:     strings.TrimSpace(participant.IssueID),
		Role:        strings.TrimSpace(participant.Role),
		Sequence:    int64(sequence),
	}); err != nil {
		return fmt.Errorf("insert issue operation participant for %q: %w", operationID, err)
	}
	return nil
}

func insertIssueLink(ctx context.Context, q *issuesdb.Queries, link IssueLink) error {
	if err := q.InsertIssueLink(ctx, issuesdb.InsertIssueLinkParams{
		ID:                strings.TrimSpace(link.ID),
		SourceIssueID:     strings.TrimSpace(link.SourceIssueID),
		TargetIssueID:     strings.TrimSpace(link.TargetIssueID),
		Type:              strings.TrimSpace(string(normalizeIssueLinkType(link.Type))),
		CreatedBy:         strings.TrimSpace(link.CreatedBy),
		CreatedAtUnixNano: link.CreatedAt.UTC().UnixNano(),
		Note:              strings.TrimSpace(link.Note),
		OperationID:       strings.TrimSpace(link.OperationID),
	}); err != nil {
		return fmt.Errorf("insert issue link %q: %w", link.ID, err)
	}
	return nil
}

func hydrateIssueOperationResult(
	ctx context.Context,
	q issueQuerier,
	operationID string,
	createdIssues []Issue,
	touched []IssueReference,
) (IssueOperationResult, error) {
	operationRows, err := q.ListIssueOperationsForIssue(ctx, touched[0].ID)
	if err != nil {
		return IssueOperationResult{}, fmt.Errorf("list issue operations for result: %w", err)
	}
	allIssueRows, err := q.ListIssues(ctx)
	if err != nil {
		return IssueOperationResult{}, fmt.Errorf("list issues for operation result: %w", err)
	}
	issuesByID := make(map[string]Issue, len(allIssueRows))
	for _, row := range allIssueRows {
		issue, err := issueFromQuery(row)
		if err != nil {
			return IssueOperationResult{}, err
		}
		issuesByID[issue.ID] = issue
	}

	for _, row := range operationRows {
		if row.ID != operationID {
			continue
		}
		participantsRows, err := q.ListIssueOperationParticipants(ctx, row.ID)
		if err != nil {
			return IssueOperationResult{}, fmt.Errorf("list operation participants: %w", err)
		}
		operation := issueOperationFromQuery(row)
		for _, participantRow := range participantsRows {
			participant := issueOperationParticipantFromQuery(participantRow)
			if related, ok := issuesByID[participant.IssueID]; ok {
				ref := issueReference(related)
				participant.Issue = &ref
			}
			operation.Participants = append(operation.Participants, participant)
		}
		return IssueOperationResult{
			Operation:     operation,
			CreatedIssues: cloneIssues(createdIssues),
			TouchedIssues: append([]IssueReference(nil), touched...),
		}, nil
	}

	return IssueOperationResult{}, fmt.Errorf("issue operation %q not found", operationID)
}

func nextSequenceBase(items []Issue) uint64 {
	var maxSeq uint64
	if length := uint64(len(items)); length > maxSeq {
		maxSeq = length
	}

	for _, issue := range items {
		seq, ok := issueSequence(issue.ID)
		if ok && seq > maxSeq {
			maxSeq = seq
		}
	}

	return maxSeq
}

func issueSequence(id string) (uint64, bool) {
	if !strings.HasPrefix(id, "issue-") {
		return 0, false
	}
	seq, err := strconv.ParseUint(strings.TrimPrefix(id, "issue-"), 10, 64)
	if err != nil {
		return 0, false
	}
	return seq, true
}

func marshalJSONB[T any](value T, empty T) (json.RawMessage, error) {
	v := any(value)
	if v == nil {
		v = empty
	}
	encoded, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func unmarshalJSONB[T any](raw json.RawMessage) (T, error) {
	var result T
	if len(raw) == 0 {
		return result, nil
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	return result, nil
}

func normalizeCatalogTags(tags []Tag) []Tag {
	if len(tags) == 0 {
		return nil
	}

	merged := make(map[string]Tag, len(tags))
	for _, raw := range tags {
		name := sanitizeTagName(raw.Name)
		if name == "" {
			continue
		}

		next := Tag{
			Name:        name,
			Description: strings.TrimSpace(raw.Description),
			CreatedAt:   raw.CreatedAt,
			Embedding:   copyEmbedding(raw.Embedding),
		}

		existing, ok := merged[name]
		if !ok {
			merged[name] = next
			continue
		}

		if existing.Description == "" && next.Description != "" {
			existing.Description = next.Description
		}
		if len(existing.Embedding) == 0 && len(next.Embedding) > 0 {
			existing.Embedding = copyEmbedding(next.Embedding)
		}
		if existing.CreatedAt.IsZero() && !next.CreatedAt.IsZero() {
			existing.CreatedAt = next.CreatedAt
		}
		merged[name] = existing
	}

	if len(merged) == 0 {
		return nil
	}

	out := make([]Tag, 0, len(merged))
	for _, tag := range merged {
		out = append(out, tag)
	}
	slices.SortStableFunc(out, func(a, b Tag) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return out
}
