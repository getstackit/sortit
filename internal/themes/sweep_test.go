package themes

import (
	"context"
	"flag"
	"fmt"
	"testing"
	"time"

	"sortit/internal/issues"
	"sortit/internal/matheval"
	"sortit/internal/ridgedecomp"
	"sortit/internal/ridgelambda"
)

var themeSweep = flag.Bool("themesweep", false, "run the WP-206 K sweep (log-only)")

// TestThemeCountSweep is the WP-206 K experiment: opt-in (-themesweep),
// log-only. For K in 4..12 it records the factorization's final reconstruction
// error and iterations-used, plus identity churn (mints + retires) across 12
// small deterministic mutations — the same stability notion the identity soak
// asserts at the default K. Two corpora: the matheval fixture (16 tags, room
// for the full K range) and the 6-group soak corpus (K clamps at its 6 tags —
// reported at the effective K). Results and the dated K decision live in
// docs/math-plan/20-stage-2-themes.md.
func TestThemeCountSweep(t *testing.T) {
	if !*themeSweep {
		t.Skip("opt-in experiment: go test ./internal/themes/ -run ThemeCountSweep -themesweep -v")
	}

	t.Logf("matheval fixture corpus (48 issues, 16 tags)")
	t.Logf("| K | reconErr | iterations | churn/12 |")
	t.Logf("|---|----------|------------|----------|")
	for k := 4; k <= 12; k++ {
		recon, iters, churn := sweepMathevalCorpus(t, k)
		t.Logf("| %d | %.4f | %d | %d |", k, recon, iters, churn)
	}

	t.Logf("soak corpus (6 groups x 8 issues, 6 tags; K clamps at 6)")
	t.Logf("| K | reconErr | iterations | churn/12 |")
	t.Logf("|---|----------|------------|----------|")
	for k := 4; k <= 8; k++ {
		recon, iters, churn := sweepSoakCorpus(t, k)
		t.Logf("| %d | %.4f | %d | %d |", k, recon, iters, churn)
	}
}

func sweepMathevalCorpus(t *testing.T, k int) (recon float64, iters, churn int) {
	t.Helper()
	corpus := matheval.GenerateCorpus()
	items := corpus.StoreIssues(time.Now())
	c, store, _, rev := sweepCache(items, corpus.StoreTags(), k)

	res, ok, err := c.Current(context.Background())
	if err != nil || !ok {
		t.Fatalf("K=%d initial compute: ok=%v err=%v", k, ok, err)
	}
	recon, iters = res.Telemetry.ReconstructionError, res.Telemetry.Iterations

	for step := 0; step < 12; step++ {
		switch step % 3 {
		case 0: // clone an existing issue under a fresh ID
			src := store.items[(step*7)%len(store.items)]
			clone := src
			clone.ID = fmt.Sprintf("sweep-add-%03d", step)
			clone.TagScores = append([]issues.TagRelevance(nil), src.TagScores...)
			store.items = append(store.items, clone)
		case 1: // nudge one issue's first tag score
			i := (step * 5) % len(store.items)
			if len(store.items[i].TagScores) > 0 {
				store.items[i].TagScores[0].Relevance *= 0.95
			}
		case 2: // drop the newest issue
			store.items = store.items[:len(store.items)-1]
		}
		rev.rev.Add(1)
		got, ok, err := c.Current(context.Background())
		if err != nil || !ok {
			t.Fatalf("K=%d refresh %d: ok=%v err=%v", k, step, ok, err)
		}
		churn += len(got.MintedIDs) + len(got.RetiredIDs)
	}
	return recon, iters, churn
}

func sweepSoakCorpus(t *testing.T, k int) (recon float64, iters, churn int) {
	t.Helper()
	const groups, perGroup = 6, 8
	c, store, _, rev := mutableCache(groupedIssues(groups, perGroup), groupTags(groups))
	c.ThemeCount = k

	res, ok, err := c.Current(context.Background())
	if err != nil || !ok {
		t.Fatalf("K=%d initial compute: ok=%v err=%v", k, ok, err)
	}
	recon, iters = res.Telemetry.ReconstructionError, res.Telemetry.Iterations

	counter := perGroup
	for step := 0; step < 12; step++ {
		grp := step % groups
		switch step % 3 {
		case 0:
			store.items = append(store.items, groupIssue(grp, counter, groups))
			counter++
		case 1:
			tweakGroupIssue(store, grp)
		case 2:
			removeOneFromGroup(store, grp)
		}
		rev.rev.Add(1)
		got, ok, err := c.Current(context.Background())
		if err != nil || !ok {
			t.Fatalf("K=%d refresh %d: ok=%v err=%v", k, step, ok, err)
		}
		churn += len(got.MintedIDs) + len(got.RetiredIDs)
	}
	return recon, iters, churn
}

func sweepCache(items []issues.Issue, tags []issues.Tag, k int) (*Cache, *stubStore, *stubTags, *stubRevisions) {
	store := &stubStore{items: items}
	tagSrc := &stubTags{tags: tags}
	rev := &stubRevisions{}
	rev.rev.Store(1)
	lambda := &ridgelambda.Cache{Store: store, Tags: tagSrc, Revisions: rev}
	decomp := &ridgedecomp.Cache{Store: store, Tags: tagSrc, Revisions: rev, Lambda: lambda}
	return &Cache{Decomp: decomp, Store: store, Revisions: rev, ThemeCount: k}, store, tagSrc, rev
}
