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
	defaultAPITokenCheckpoint = "api-token-backfill"
	apiTokenDomain            = "api-tokens"

	// apiTokenKindCreated/apiTokenKindRevoked mirror the runtime fact kinds
	// written by internal/auth on token create/revoke.
	apiTokenKindCreated = "created"
	apiTokenKindRevoked = "revoked"
)

// APITokenBackfillResult summarizes one batch of the api_token_facts backfill.
type APITokenBackfillResult struct {
	Checkpoint      string `json:"checkpoint"`
	ProcessedTokens int    `json:"processedTokens"`
	WroteFacts      int    `json:"wroteFacts"`
	LastTokenID     string `json:"lastTokenId,omitempty"`
	Complete        bool   `json:"complete"`
}

// APITokenParityMismatch records a single token whose api_token_facts history does
// not agree with the api_tokens projection.
type APITokenParityMismatch struct {
	TokenID string   `json:"tokenId"`
	Fields  []string `json:"fields"`
}

// APITokenParityResult reports how well the append-only facts reproduce the
// api_tokens current-state projection.
type APITokenParityResult struct {
	Domain           string                   `json:"domain"`
	TotalTokens      int                      `json:"totalTokens"`
	TokensWithFacts  int                      `json:"tokensWithFacts"`
	MissingFacts     int                      `json:"missingFacts"`
	MismatchedTokens int                      `json:"mismatchedTokens"`
	Mismatches       []APITokenParityMismatch `json:"mismatches,omitempty"`
}

type APITokenStore struct {
	db *sql.DB
}

func NewAPITokenStore(db *sql.DB) *APITokenStore {
	return &APITokenStore{db: db}
}

type apiTokenLegacyRow struct {
	ID          string
	UserID      string
	Name        string
	TokenPrefix string
	CreatedAt   int64
	RevokedAt   int64
}

type apiTokenCursor struct {
	LastTokenID string `json:"lastTokenId"`
}

type apiTokenFactSummary struct {
	HasCreated bool
	HasRevoked bool
}

// BackfillBatch seeds api_token_facts from existing api_tokens rows: one 'created'
// fact per token plus a 'revoked' fact for tokens already revoked. It is
// idempotent and additive — a token that already has a fact of a given kind (for
// example one written by the runtime create/revoke path) is left untouched, which
// also avoids colliding with the unique (token_id, sequence) index.
func (s *APITokenStore) BackfillBatch(
	ctx context.Context,
	checkpointName string,
	batchSize int,
) (APITokenBackfillResult, error) {
	checkpointName = strings.TrimSpace(checkpointName)
	if checkpointName == "" {
		checkpointName = defaultAPITokenCheckpoint
	}
	if batchSize <= 0 {
		batchSize = 100
	}

	framework := NewFrameworkStore(s.db)
	checkpoint, ok, err := framework.GetCheckpoint(ctx, checkpointName)
	if err != nil {
		return APITokenBackfillResult{}, err
	}

	cursor := apiTokenCursor{}
	if ok && len(checkpoint.CursorJSON) > 0 {
		if err := json.Unmarshal(checkpoint.CursorJSON, &cursor); err != nil {
			return APITokenBackfillResult{}, fmt.Errorf("decode api token checkpoint cursor: %w", err)
		}
	}

	tokens, err := s.listLegacyTokens(ctx)
	if err != nil {
		return APITokenBackfillResult{}, err
	}

	candidates := make([]apiTokenLegacyRow, 0, batchSize)
	remainingCount := 0
	for _, token := range tokens {
		if cursor.LastTokenID != "" && token.ID <= cursor.LastTokenID {
			continue
		}
		remainingCount++
		if len(candidates) < batchSize {
			candidates = append(candidates, token)
		}
	}

	result := APITokenBackfillResult{
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
			return APITokenBackfillResult{}, err
		}
		return result, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return APITokenBackfillResult{}, fmt.Errorf("begin api token backfill tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, token := range candidates {
		summary, err := loadTokenFactSummary(ctx, tx, token.ID)
		if err != nil {
			return APITokenBackfillResult{}, err
		}

		if !summary.HasCreated {
			if err := s.insertBackfillFact(ctx, tx, token, apiTokenKindCreated); err != nil {
				return APITokenBackfillResult{}, err
			}
			result.WroteFacts++
		}
		if token.RevokedAt > 0 && !summary.HasRevoked {
			if err := s.insertBackfillFact(ctx, tx, token, apiTokenKindRevoked); err != nil {
				return APITokenBackfillResult{}, err
			}
			result.WroteFacts++
		}

		result.ProcessedTokens++
		result.LastTokenID = token.ID
	}

	if err := tx.Commit(); err != nil {
		return APITokenBackfillResult{}, fmt.Errorf("commit api token backfill tx: %w", err)
	}

	result.Complete = remainingCount <= batchSize
	cursor.LastTokenID = result.LastTokenID
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
		return APITokenBackfillResult{}, err
	}

	return result, nil
}

// CheckParity verifies that every api_tokens row has a derivable history in
// api_token_facts: a 'created' fact must exist, and the presence of a 'revoked'
// fact must agree with the projection's revoked state.
func (s *APITokenStore) CheckParity(ctx context.Context, detailLimit int) (APITokenParityResult, error) {
	if detailLimit <= 0 {
		detailLimit = 20
	}

	tokens, err := s.listLegacyTokens(ctx)
	if err != nil {
		return APITokenParityResult{}, err
	}
	summaries, err := s.loadAllFactSummaries(ctx)
	if err != nil {
		return APITokenParityResult{}, err
	}

	result := APITokenParityResult{
		Domain:          apiTokenDomain,
		TotalTokens:     len(tokens),
		TokensWithFacts: len(summaries),
	}

	for _, token := range tokens {
		summary := summaries[token.ID]
		fields := make([]string, 0, 2)
		if !summary.HasCreated {
			fields = append(fields, "missing_created_fact")
		}
		if (token.RevokedAt > 0) != summary.HasRevoked {
			fields = append(fields, "revoked")
		}
		if len(fields) == 0 {
			continue
		}
		if !summary.HasCreated {
			result.MissingFacts++
		}
		result.MismatchedTokens++
		if len(result.Mismatches) < detailLimit {
			result.Mismatches = append(result.Mismatches, APITokenParityMismatch{
				TokenID: token.ID,
				Fields:  fields,
			})
		}
	}

	return result, nil
}

func (s *APITokenStore) insertBackfillFact(
	ctx context.Context,
	tx *sql.Tx,
	token apiTokenLegacyRow,
	kind string,
) error {
	var (
		factID    string
		sequence  int64
		createdAt int64
		sourceID  string
		payload   map[string]any
	)
	switch kind {
	case apiTokenKindCreated:
		factID = "backfill-api-token-created:" + token.ID
		sequence = 1
		createdAt = token.CreatedAt
		sourceID = token.ID + ":created"
		payload = map[string]any{
			"name":              token.Name,
			"tokenPrefix":       token.TokenPrefix,
			"createdAtUnixNano": token.CreatedAt,
		}
	case apiTokenKindRevoked:
		next, err := nextBackfillSequence(ctx, tx, token.ID)
		if err != nil {
			return err
		}
		factID = "backfill-api-token-revoked:" + token.ID
		sequence = next
		createdAt = token.RevokedAt
		sourceID = token.ID + ":revoked"
		payload = map[string]any{
			"revokedAtUnixNano": token.RevokedAt,
		}
	default:
		return fmt.Errorf("unsupported api token fact kind %q", kind)
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO api_token_facts (
		     id,
		     token_id,
		     user_id,
		     sequence,
		     kind,
		     created_by,
		     created_at_unix_nano,
		     payload_json,
		     source,
		     source_id,
		     inferred
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10, $11)
		 ON CONFLICT (source, source_id) DO NOTHING`,
		factID,
		token.ID,
		token.UserID,
		sequence,
		kind,
		"",
		createdAt,
		[]byte(mustMarshalJSON(payload)),
		"legacy_api_token",
		sourceID,
		true,
	); err != nil {
		return fmt.Errorf("insert backfill api token %s fact for %q: %w", kind, token.ID, err)
	}
	return nil
}

func (s *APITokenStore) listLegacyTokens(ctx context.Context) ([]apiTokenLegacyRow, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, user_id, name, token_prefix, created_at_unix_nano, revoked_at_unix_nano
		 FROM api_tokens`,
	)
	if err != nil {
		return nil, fmt.Errorf("list api tokens for backfill: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var tokens []apiTokenLegacyRow
	for rows.Next() {
		var token apiTokenLegacyRow
		if err := rows.Scan(
			&token.ID,
			&token.UserID,
			&token.Name,
			&token.TokenPrefix,
			&token.CreatedAt,
			&token.RevokedAt,
		); err != nil {
			return nil, fmt.Errorf("scan api token row: %w", err)
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api token rows: %w", err)
	}

	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].ID < tokens[j].ID
	})
	return tokens, nil
}

func (s *APITokenStore) loadAllFactSummaries(ctx context.Context) (map[string]apiTokenFactSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT token_id, kind FROM api_token_facts`)
	if err != nil {
		return nil, fmt.Errorf("list api token facts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	summaries := make(map[string]apiTokenFactSummary)
	for rows.Next() {
		var tokenID, kind string
		if err := rows.Scan(&tokenID, &kind); err != nil {
			return nil, fmt.Errorf("scan api token fact: %w", err)
		}
		summary := summaries[tokenID]
		applyFactKind(&summary, kind)
		summaries[tokenID] = summary
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api token facts: %w", err)
	}
	return summaries, nil
}

func loadTokenFactSummary(ctx context.Context, tx *sql.Tx, tokenID string) (apiTokenFactSummary, error) {
	rows, err := tx.QueryContext(ctx, `SELECT kind FROM api_token_facts WHERE token_id = $1`, tokenID)
	if err != nil {
		return apiTokenFactSummary{}, fmt.Errorf("load api token fact kinds for %q: %w", tokenID, err)
	}
	defer rows.Close() //nolint:errcheck

	var summary apiTokenFactSummary
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			return apiTokenFactSummary{}, fmt.Errorf("scan api token fact kind for %q: %w", tokenID, err)
		}
		applyFactKind(&summary, kind)
	}
	if err := rows.Err(); err != nil {
		return apiTokenFactSummary{}, fmt.Errorf("iterate api token fact kinds for %q: %w", tokenID, err)
	}
	return summary, nil
}

func applyFactKind(summary *apiTokenFactSummary, kind string) {
	switch strings.TrimSpace(kind) {
	case apiTokenKindCreated:
		summary.HasCreated = true
	case apiTokenKindRevoked:
		summary.HasRevoked = true
	}
}

func nextBackfillSequence(ctx context.Context, tx *sql.Tx, tokenID string) (int64, error) {
	var sequence int64
	if err := tx.QueryRowContext(
		ctx,
		`SELECT COALESCE(MAX(sequence), 0) + 1 FROM api_token_facts WHERE token_id = $1`,
		tokenID,
	).Scan(&sequence); err != nil {
		return 0, fmt.Errorf("load next api token fact sequence for %q: %w", tokenID, err)
	}
	return sequence, nil
}
