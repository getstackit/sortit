package appendonly

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	defaultUserProfileCheckpoint = "user-profile-backfill"
	userProfileDomain            = "user-profiles"
)

// UserProfileBackfillResult summarizes one batch of the user_profile_facts backfill.
type UserProfileBackfillResult struct {
	Checkpoint     string `json:"checkpoint"`
	ProcessedUsers int    `json:"processedUsers"`
	WroteFacts     int    `json:"wroteFacts"`
	LastUserID     string `json:"lastUserId,omitempty"`
	Complete       bool   `json:"complete"`
}

// UserProfileParityMismatch records a single user whose user_profile_facts history
// does not agree with the users projection.
type UserProfileParityMismatch struct {
	UserID string   `json:"userId"`
	Fields []string `json:"fields"`
}

// UserProfileParityResult reports how well the append-only facts reproduce the
// users latest-profile projection.
type UserProfileParityResult struct {
	Domain          string                      `json:"domain"`
	TotalUsers      int                         `json:"totalUsers"`
	UsersWithFacts  int                         `json:"usersWithFacts"`
	MissingFacts    int                         `json:"missingFacts"`
	MismatchedUsers int                         `json:"mismatchedUsers"`
	Mismatches      []UserProfileParityMismatch `json:"mismatches,omitempty"`
}

type UserProfileStore struct {
	db *sql.DB
}

func NewUserProfileStore(db *sql.DB) *UserProfileStore {
	return &UserProfileStore{db: db}
}

type userProfileRow struct {
	UserID      string
	Login       string
	DisplayName string
	AvatarURL   string
	Email       string
	UpdatedAt   int64
}

type userProfileCursor struct {
	LastUserID string `json:"lastUserId"`
}

// BackfillBatch seeds user_profile_facts from existing users rows: one initial
// snapshot fact per user. It is idempotent and additive — a user that already has
// a fact (for example one written by the runtime login path) is left untouched,
// which also avoids colliding with the unique (user_id, sequence) index.
func (s *UserProfileStore) BackfillBatch(
	ctx context.Context,
	checkpointName string,
	batchSize int,
) (UserProfileBackfillResult, error) {
	checkpointName = strings.TrimSpace(checkpointName)
	if checkpointName == "" {
		checkpointName = defaultUserProfileCheckpoint
	}
	if batchSize <= 0 {
		batchSize = 100
	}

	framework := NewFrameworkStore(s.db)
	checkpoint, ok, err := framework.GetCheckpoint(ctx, checkpointName)
	if err != nil {
		return UserProfileBackfillResult{}, err
	}

	cursor := userProfileCursor{}
	if ok && len(checkpoint.CursorJSON) > 0 {
		if err := json.Unmarshal(checkpoint.CursorJSON, &cursor); err != nil {
			return UserProfileBackfillResult{}, fmt.Errorf("decode user profile checkpoint cursor: %w", err)
		}
	}

	users, err := s.listUsers(ctx)
	if err != nil {
		return UserProfileBackfillResult{}, err
	}

	candidates := make([]userProfileRow, 0, batchSize)
	remainingCount := 0
	for _, user := range users {
		if cursor.LastUserID != "" && user.UserID <= cursor.LastUserID {
			continue
		}
		remainingCount++
		if len(candidates) < batchSize {
			candidates = append(candidates, user)
		}
	}

	result := UserProfileBackfillResult{
		Checkpoint: checkpointName,
		Complete:   len(candidates) == 0,
	}
	if len(candidates) == 0 {
		if err := framework.UpsertCheckpoint(ctx, Checkpoint{
			Name:        checkpointName,
			Phase:       phaseCompleted,
			CursorJSON:  mustMarshalJSON(cursor),
			SummaryJSON: mustMarshalJSON(result),
		}); err != nil {
			return UserProfileBackfillResult{}, err
		}
		return result, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UserProfileBackfillResult{}, fmt.Errorf("begin user profile backfill tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, user := range candidates {
		hasFacts, err := userHasProfileFacts(ctx, tx, user.UserID)
		if err != nil {
			return UserProfileBackfillResult{}, err
		}
		if !hasFacts {
			if err := insertBackfillProfileFact(ctx, tx, user); err != nil {
				return UserProfileBackfillResult{}, err
			}
			result.WroteFacts++
		}
		result.ProcessedUsers++
		result.LastUserID = user.UserID
	}

	if err := tx.Commit(); err != nil {
		return UserProfileBackfillResult{}, fmt.Errorf("commit user profile backfill tx: %w", err)
	}

	result.Complete = remainingCount <= batchSize
	cursor.LastUserID = result.LastUserID
	phase := phaseBackfilling
	if result.Complete {
		phase = phaseCompleted
	}
	if err := framework.UpsertCheckpoint(ctx, Checkpoint{
		Name:        checkpointName,
		Phase:       phase,
		CursorJSON:  mustMarshalJSON(cursor),
		SummaryJSON: mustMarshalJSON(result),
	}); err != nil {
		return UserProfileBackfillResult{}, err
	}

	return result, nil
}

// CheckParity verifies that the latest user_profile_facts snapshot for each user
// reproduces the users projection (login / display_name / avatar_url / email).
func (s *UserProfileStore) CheckParity(ctx context.Context, detailLimit int) (UserProfileParityResult, error) {
	if detailLimit <= 0 {
		detailLimit = 20
	}

	users, err := s.listUsers(ctx)
	if err != nil {
		return UserProfileParityResult{}, err
	}
	latest, err := s.loadLatestFacts(ctx)
	if err != nil {
		return UserProfileParityResult{}, err
	}

	result := UserProfileParityResult{
		Domain:         userProfileDomain,
		TotalUsers:     len(users),
		UsersWithFacts: len(latest),
	}

	for _, user := range users {
		fact, ok := latest[user.UserID]
		if !ok {
			result.MissingFacts++
			result.MismatchedUsers++
			if len(result.Mismatches) < detailLimit {
				result.Mismatches = append(result.Mismatches, UserProfileParityMismatch{
					UserID: user.UserID,
					Fields: []string{"missing_fact"},
				})
			}
			continue
		}

		fields := compareUserProfile(user, fact)
		if len(fields) == 0 {
			continue
		}
		result.MismatchedUsers++
		if len(result.Mismatches) < detailLimit {
			result.Mismatches = append(result.Mismatches, UserProfileParityMismatch{
				UserID: user.UserID,
				Fields: fields,
			})
		}
	}

	return result, nil
}

func compareUserProfile(user userProfileRow, fact userProfileRow) []string {
	fields := make([]string, 0, 4)
	if user.Login != fact.Login {
		fields = append(fields, "login")
	}
	if user.DisplayName != fact.DisplayName {
		fields = append(fields, "display_name")
	}
	if user.AvatarURL != fact.AvatarURL {
		fields = append(fields, "avatar_url")
	}
	if user.Email != fact.Email {
		fields = append(fields, "email")
	}
	return fields
}

func insertBackfillProfileFact(ctx context.Context, tx *sql.Tx, user userProfileRow) error {
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO user_profile_facts (
		     id,
		     user_id,
		     sequence,
		     login,
		     display_name,
		     avatar_url,
		     email,
		     observed_at_unix_nano,
		     source,
		     source_id,
		     inferred
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT (source, source_id) DO NOTHING`,
		"backfill-user-profile:"+user.UserID,
		user.UserID,
		1,
		user.Login,
		user.DisplayName,
		user.AvatarURL,
		user.Email,
		user.UpdatedAt,
		"legacy_user_profile",
		user.UserID,
		true,
	); err != nil {
		return fmt.Errorf("insert backfill user profile fact for %q: %w", user.UserID, err)
	}
	return nil
}

func (s *UserProfileStore) listUsers(ctx context.Context) ([]userProfileRow, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, login, display_name, avatar_url, email, updated_at_unix_nano FROM users`,
	)
	if err != nil {
		return nil, fmt.Errorf("list users for backfill: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var users []userProfileRow
	for rows.Next() {
		var user userProfileRow
		if err := rows.Scan(
			&user.UserID,
			&user.Login,
			&user.DisplayName,
			&user.AvatarURL,
			&user.Email,
			&user.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan user row: %w", err)
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user rows: %w", err)
	}

	sort.Slice(users, func(i, j int) bool {
		return users[i].UserID < users[j].UserID
	})
	return users, nil
}

func (s *UserProfileStore) loadLatestFacts(ctx context.Context) (map[string]userProfileRow, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT DISTINCT ON (user_id) user_id, login, display_name, avatar_url, email
		 FROM user_profile_facts
		 ORDER BY user_id, sequence DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list latest user profile facts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	latest := make(map[string]userProfileRow)
	for rows.Next() {
		var fact userProfileRow
		if err := rows.Scan(
			&fact.UserID,
			&fact.Login,
			&fact.DisplayName,
			&fact.AvatarURL,
			&fact.Email,
		); err != nil {
			return nil, fmt.Errorf("scan user profile fact: %w", err)
		}
		latest[fact.UserID] = fact
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user profile facts: %w", err)
	}
	return latest, nil
}

func userHasProfileFacts(ctx context.Context, tx *sql.Tx, userID string) (bool, error) {
	var exists bool
	if err := tx.QueryRowContext(
		ctx,
		`SELECT EXISTS (SELECT 1 FROM user_profile_facts WHERE user_id = $1)`,
		userID,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("check user profile facts for %q: %w", userID, err)
	}
	return exists, nil
}
