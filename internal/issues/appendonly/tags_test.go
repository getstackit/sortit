package appendonly

import (
	"context"
	"testing"
	"time"
)

func TestTagBackfillRebuildsProjectionFromLegacyTagsAndMerges(t *testing.T) {
	ctx := context.Background()
	store := newAppendonlyTestStore(t, ctx)
	tagStore := NewTagStore(store.DB())

	createdAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	computedAt := createdAt.Add(15 * time.Minute)
	if _, err := store.DB().ExecContext(
		ctx,
		`INSERT INTO tags (
		     name,
		     description,
		     created_at_unix_nano,
		     embedding_vector,
		     specificity,
		     specificity_llm,
		     specificity_embedding,
		     specificity_computed_at
		 ) VALUES ($1, $2, $3, $4::vector, $5, $6, $7, $8)`,
		"bug",
		"software defect",
		createdAt.UnixNano(),
		"[0.1,0.9]",
		0.8,
		0.85,
		0.75,
		computedAt,
	); err != nil {
		t.Fatalf("insert legacy tag: %v", err)
	}
	if _, err := store.DB().ExecContext(
		ctx,
		`INSERT INTO tag_merge_history (canonical_name, alias_name, merged_at, merged_by)
		 VALUES ($1, $2, $3, $4)`,
		"bug",
		"defect",
		createdAt.Add(30*time.Minute),
		"Casey",
	); err != nil {
		t.Fatalf("insert tag merge history: %v", err)
	}

	result, err := tagStore.BackfillBatch(ctx, "tag-backfill-test", 100)
	if err != nil {
		t.Fatalf("backfill tags: %v", err)
	}
	if result.ProcessedTags != 1 || result.ProcessedMerges != 1 {
		t.Fatalf("unexpected tag backfill counts: %#v", result)
	}

	parity, err := tagStore.CheckParity(ctx, 10)
	if err != nil {
		t.Fatalf("check tag parity: %v", err)
	}
	if parity.MissingTags != 0 || parity.MismatchedTags != 0 || parity.MissingMergedAliases != 0 {
		t.Fatalf("expected tag parity match, got %#v", parity)
	}

	var (
		status        string
		canonicalName string
	)
	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT status, canonical_name
		 FROM tag_projections
		 WHERE name = $1`,
		"defect",
	).Scan(&status, &canonicalName); err != nil {
		t.Fatalf("read alias tag projection: %v", err)
	}
	if status != "merged" || canonicalName != "bug" {
		t.Fatalf("expected merged alias projection defect->bug, got status=%q canonical=%q", status, canonicalName)
	}
}
