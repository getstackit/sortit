package themes

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"sortit/internal/matheval"
	"sortit/internal/ridgedecomp"
	"sortit/internal/ridgelambda"
)

// TestQualitativeThemeCoherence is the WP-204 fixture-tier stand-in for the
// spec's dev-corpus sanity read (a live server + seeded corpus aren't
// available at debug tier). It runs the matheval fixture corpus — 48 issues,
// 16 tags in 8 correlated domain pairs (money: billing/payments, identity:
// auth/login, documents: export/pdf, discovery: search, performance,
// mobile: mobile/ios, communications: notifications/email, data:
// database/migration, plus two generic UI/backend tags) — through the real
// themes cache and logs the theme list (stable ID, top tags, weight share) so
// a human reviewer can read it directly in `go test -v` output.
//
// This is not a pass/fail assertion on theme *content* — NMF theme quality is
// a judgment call, not a numeric threshold (that is WP-206's job, once
// telemetry exists). It only asserts the factorization is usable (computed,
// themes present) and prints the theme list for the required qualitative
// read, which is recorded in the WP-204 PR description per the plan.
func TestQualitativeThemeCoherence(t *testing.T) {
	corpus := matheval.GenerateCorpus()
	now := time.Now()
	items := corpus.StoreIssues(now)
	storeTags := corpus.StoreTags()

	store := &stubStore{items: items}
	tagSrc := &stubTags{tags: storeTags}
	rev := &stubRevisions{}
	rev.rev.Store(1)
	lambda := &ridgelambda.Cache{Store: store, Tags: tagSrc, Revisions: rev}
	decomp := &ridgedecomp.Cache{Store: store, Tags: tagSrc, Revisions: rev, Lambda: lambda}
	cache := &Cache{Decomp: decomp, Store: store, Revisions: rev}

	res, ok, err := cache.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if !ok {
		t.Fatalf("expected a usable factorization over the %d-issue matheval fixture corpus (theme floor is %d) — this is a WP-206 trigger, not a test bug", len(items), minThemeParticipants)
	}
	if len(res.Themes) == 0 {
		t.Fatal("expected at least one theme")
	}

	t.Logf("matheval fixture corpus: %d issues, %d tags, %d participating (%d excluded-no-embedding, %d excluded-no-anchor)",
		len(items), len(storeTags), res.Participating, res.ExcludedNoEmbedding, res.ExcludedNoAnchor)
	t.Logf("%d themes, %d minted, %d retired", len(res.Themes), len(res.MintedIDs), len(res.RetiredIDs))

	totalWeight := 0.0
	for _, theme := range res.Themes {
		totalWeight += theme.Weight
	}
	for i, theme := range res.Themes {
		id := ""
		if i < len(res.Identities) {
			id = res.Identities[i].StableID
		}
		share := 0.0
		if totalWeight > 0 {
			share = theme.Weight / totalWeight
		}
		tagNames := make([]string, len(theme.Tags))
		for j, tag := range theme.Tags {
			tagNames[j] = fmt.Sprintf("%s(%.2f)", tag.Tag, tag.Loading)
		}
		t.Logf("  %-10s weight=%.3f tags=[%s]", id, share, strings.Join(tagNames, ", "))
	}
}
