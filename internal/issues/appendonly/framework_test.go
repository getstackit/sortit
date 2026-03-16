package appendonly

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"splat/internal/issues"
	"splat/internal/testpostgres"
)

func TestFrameworkStoreCheckpointAndParityRunCRUD(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	harness, err := testpostgres.Start(ctx, "splat_appendonly_framework_test")
	if err != nil {
		t.Fatalf("start postgres harness: %v", err)
	}
	t.Cleanup(func() {
		if err := harness.Terminate(ctx); err != nil {
			t.Fatalf("terminate postgres harness: %v", err)
		}
	})

	databaseURL := harness.Acquire(t, func(ctx context.Context, databaseURL string) error {
		store, err := issues.OpenPostgresStore(ctx, databaseURL)
		if err != nil {
			return err
		}
		return store.Close()
	})

	store, err := issues.OpenPostgresStore(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close postgres store: %v", err)
		}
	})

	framework := NewFrameworkStore(store.DB())

	checkpointTime := time.Now().UTC().Truncate(time.Microsecond)
	if err := framework.UpsertCheckpoint(ctx, Checkpoint{
		Name:        "issue-lifecycle-backfill",
		Phase:       "planned",
		CursorJSON:  json.RawMessage(`{"issueId":"issue-000123"}`),
		SummaryJSON: json.RawMessage(`{"rowsProcessed":12}`),
		UpdatedAt:   checkpointTime,
	}); err != nil {
		t.Fatalf("upsert checkpoint: %v", err)
	}

	checkpoints, err := framework.ListCheckpoints(ctx)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if len(checkpoints) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(checkpoints))
	}
	if checkpoints[0].Name != "issue-lifecycle-backfill" {
		t.Fatalf("unexpected checkpoint name %q", checkpoints[0].Name)
	}
	if !jsonEqual(checkpoints[0].CursorJSON, json.RawMessage(`{"issueId":"issue-000123"}`)) {
		t.Fatalf("unexpected checkpoint cursor %s", checkpoints[0].CursorJSON)
	}

	parityTime := checkpointTime.Add(time.Second)
	if err := framework.RecordParityRun(ctx, ParityRun{
		ID:          "parity-000001",
		Domain:      "issue-lifecycle",
		Status:      "baseline",
		DetailsJSON: json.RawMessage(`{"issues":42,"open":35}`),
		CreatedAt:   parityTime,
	}); err != nil {
		t.Fatalf("record parity run: %v", err)
	}

	runs, err := framework.ListParityRuns(ctx, "issue-lifecycle", 10)
	if err != nil {
		t.Fatalf("list parity runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 parity run, got %d", len(runs))
	}
	if runs[0].ID != "parity-000001" {
		t.Fatalf("unexpected parity run id %q", runs[0].ID)
	}
	if !jsonEqual(runs[0].DetailsJSON, json.RawMessage(`{"issues":42,"open":35}`)) {
		t.Fatalf("unexpected parity run details %s", runs[0].DetailsJSON)
	}
}

func jsonEqual(left, right json.RawMessage) bool {
	var leftDecoded any
	if err := json.Unmarshal(left, &leftDecoded); err != nil {
		return false
	}

	var rightDecoded any
	if err := json.Unmarshal(right, &rightDecoded); err != nil {
		return false
	}

	leftNormalized, err := json.Marshal(leftDecoded)
	if err != nil {
		return false
	}
	rightNormalized, err := json.Marshal(rightDecoded)
	if err != nil {
		return false
	}

	return string(leftNormalized) == string(rightNormalized)
}
