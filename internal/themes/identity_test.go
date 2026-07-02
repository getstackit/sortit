package themes

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"sortit/internal/issues"
	"sortit/internal/ridgedecomp"
	"sortit/internal/ridgelambda"
)

// --- Matcher unit tests (pure, over constructed H rows) ---

// prevThemes builds the previous-revision identity list from (id, row) pairs.
func prevThemes(pairs ...struct {
	id  string
	row themeRow
}) []identifiedTheme {
	out := make([]identifiedTheme, len(pairs))
	for i, p := range pairs {
		out[i] = identifiedTheme{id: p.id, row: p.row}
	}
	return out
}

func idt(id string, row themeRow) struct {
	id  string
	row themeRow
} {
	return struct {
		id  string
		row themeRow
	}{id, row}
}

// TestMatchThemesIdentityPermutation: shuffled (and slightly jittered) rows must
// all match their counterparts at ~1.0 with the stable IDs following the rows,
// never the input order — permutation alone can never change an ID.
func TestMatchThemesIdentityPermutation(t *testing.T) {
	a := themeRow{"a": 0.6, "a2": 0.4}
	b := themeRow{"b": 0.7, "b2": 0.3}
	c := themeRow{"c": 1.0}
	prev := prevThemes(idt("theme-1", a), idt("theme-2", b), idt("theme-3", c))

	// New rows are the same themes, jittered and shuffled to order c, a, b.
	newRows := []themeRow{
		{"c": 0.99, "cx": 0.01},
		{"a": 0.61, "a2": 0.39},
		{"b": 0.69, "b2": 0.31},
	}
	res := matchThemes(prev, newRows, 3)

	want := []string{"theme-3", "theme-1", "theme-2"}
	if !reflect.DeepEqual(res.ids, want) {
		t.Fatalf("ids = %v, want %v (IDs must follow rows, not input order)", res.ids, want)
	}
	for i, s := range res.scores {
		if s < 0.98 {
			t.Fatalf("row %d matched at %.4f, expected ~1.0", i, s)
		}
	}
	if len(res.minted) != 0 || len(res.retired) != 0 {
		t.Fatalf("permutation must not mint or retire; minted=%v retired=%v", res.minted, res.retired)
	}
	if res.counter != 3 {
		t.Fatalf("counter must be untouched when nothing mints, got %d", res.counter)
	}
}

// TestMatchThemesSplit: one old theme ≈ two new rows — the best match inherits,
// the other mints.
func TestMatchThemesSplit(t *testing.T) {
	prev := prevThemes(idt("theme-1", themeRow{"x": 1}))
	newRows := []themeRow{
		{"x": 1.0},             // exact
		{"x": 0.98, "y": 0.02}, // near, but slightly worse
	}
	res := matchThemes(prev, newRows, 1)

	if res.ids[0] != "theme-1" {
		t.Fatalf("exact match should inherit theme-1, got %s", res.ids[0])
	}
	if res.ids[1] != "theme-2" {
		t.Fatalf("second split-off should mint theme-2, got %s", res.ids[1])
	}
	if !reflect.DeepEqual(res.minted, []string{"theme-2"}) {
		t.Fatalf("minted = %v, want [theme-2]", res.minted)
	}
	if len(res.retired) != 0 {
		t.Fatalf("split must not retire, got %v", res.retired)
	}
}

// TestMatchThemesMerge: two old themes collapse into one new row — the more
// similar predecessor is inherited, the other retires.
func TestMatchThemesMerge(t *testing.T) {
	prev := prevThemes(
		idt("theme-1", themeRow{"x": 1}),
		idt("theme-2", themeRow{"y": 1}),
	)
	newRows := []themeRow{{"x": 0.8, "y": 0.2}} // closer to theme-1
	res := matchThemes(prev, newRows, 2)

	if res.ids[0] != "theme-1" {
		t.Fatalf("merged row should inherit the closer theme-1, got %s", res.ids[0])
	}
	if len(res.minted) != 0 {
		t.Fatalf("merge must not mint, got %v", res.minted)
	}
	if !reflect.DeepEqual(res.retired, []string{"theme-2"}) {
		t.Fatalf("retired = %v, want [theme-2]", res.retired)
	}
}

// TestMatchThemesBelowThreshold: the only available match is below
// themeMatchThreshold, so the new theme mints and the old one retires rather than
// a bad ID being inherited.
func TestMatchThemesBelowThreshold(t *testing.T) {
	prev := prevThemes(idt("theme-1", themeRow{"x": 1}))
	// cosine = 0.5 / sqrt(0.25 + 0.75) = 0.5 < 0.6
	newRows := []themeRow{{"x": 0.5, "y": 0.866}}
	res := matchThemes(prev, newRows, 1)

	if res.ids[0] != "theme-2" {
		t.Fatalf("below-threshold match should mint theme-2, got %s (score %.4f)", res.ids[0], res.scores[0])
	}
	if res.scores[0] != 0 {
		t.Fatalf("a minted theme has no predecessor score, got %.4f", res.scores[0])
	}
	if !reflect.DeepEqual(res.minted, []string{"theme-2"}) {
		t.Fatalf("minted = %v, want [theme-2]", res.minted)
	}
	if !reflect.DeepEqual(res.retired, []string{"theme-1"}) {
		t.Fatalf("retired = %v, want [theme-1]", res.retired)
	}
}

// TestMatchThemesBoundary asserts the threshold is inclusive: a match exactly at
// themeMatchThreshold inherits, one a hair below mints.
func TestMatchThemesBoundary(t *testing.T) {
	// Construct rows with a known cosine. row a = {p:1}; row b weighted so the
	// cosine equals a target t: b = {p:t, q:sqrt(1-t^2)} gives cosine exactly t
	// against {p:1} (since |a|=1, dot=t, |b|=1).
	rowAt := func(target float64) themeRow {
		q := 0.0
		if 1-target*target > 0 {
			q = sqrtPos(1 - target*target)
		}
		return themeRow{"p": target, "q": q}
	}
	prev := prevThemes(idt("theme-1", themeRow{"p": 1}))

	at := matchThemes(prev, []themeRow{rowAt(themeMatchThreshold)}, 1)
	if at.ids[0] != "theme-1" {
		t.Fatalf("cosine == threshold must inherit (inclusive), got %s score %.6f", at.ids[0], at.scores[0])
	}

	below := matchThemes(prev, []themeRow{rowAt(themeMatchThreshold - 0.01)}, 1)
	if below.ids[0] != "theme-2" {
		t.Fatalf("cosine just below threshold must mint, got %s score %.6f", below.ids[0], below.scores[0])
	}
}

// TestMatchThemesEmptyPrev: first run — every theme mints from the counter.
func TestMatchThemesEmptyPrev(t *testing.T) {
	newRows := []themeRow{{"x": 1}, {"y": 1}, {"z": 1}}
	res := matchThemes(nil, newRows, 0)

	want := []string{"theme-1", "theme-2", "theme-3"}
	if !reflect.DeepEqual(res.ids, want) {
		t.Fatalf("ids = %v, want %v", res.ids, want)
	}
	if !reflect.DeepEqual(res.minted, want) {
		t.Fatalf("minted = %v, want %v", res.minted, want)
	}
	if len(res.retired) != 0 || res.counter != 3 {
		t.Fatalf("empty prev: retired=%v counter=%d", res.retired, res.counter)
	}
}

// TestMatchThemesKChangeUp: K grows — extant themes keep IDs, the new one mints.
func TestMatchThemesKChangeUp(t *testing.T) {
	prev := prevThemes(idt("theme-1", themeRow{"x": 1}), idt("theme-2", themeRow{"y": 1}))
	newRows := []themeRow{{"x": 1}, {"y": 1}, {"z": 1}}
	res := matchThemes(prev, newRows, 2)

	if !reflect.DeepEqual(res.ids, []string{"theme-1", "theme-2", "theme-3"}) {
		t.Fatalf("ids = %v, want [theme-1 theme-2 theme-3]", res.ids)
	}
	if !reflect.DeepEqual(res.minted, []string{"theme-3"}) {
		t.Fatalf("minted = %v, want [theme-3]", res.minted)
	}
	if len(res.retired) != 0 {
		t.Fatalf("K-up must not retire, got %v", res.retired)
	}
}

// TestMatchThemesKChangeDown: K shrinks — surviving themes keep IDs, the lost one
// retires.
func TestMatchThemesKChangeDown(t *testing.T) {
	prev := prevThemes(
		idt("theme-1", themeRow{"x": 1}),
		idt("theme-2", themeRow{"y": 1}),
		idt("theme-3", themeRow{"z": 1}),
	)
	newRows := []themeRow{{"x": 1}, {"y": 1}}
	res := matchThemes(prev, newRows, 3)

	if !reflect.DeepEqual(res.ids, []string{"theme-1", "theme-2"}) {
		t.Fatalf("ids = %v, want [theme-1 theme-2]", res.ids)
	}
	if len(res.minted) != 0 {
		t.Fatalf("K-down must not mint, got %v", res.minted)
	}
	if !reflect.DeepEqual(res.retired, []string{"theme-3"}) {
		t.Fatalf("retired = %v, want [theme-3]", res.retired)
	}
}

// TestMatchThemesDeterministic: identical inputs (including counter) yield
// identical output, and the counter never reuses an ID.
func TestMatchThemesDeterministic(t *testing.T) {
	prev := prevThemes(idt("theme-1", themeRow{"x": 1}))
	newRows := []themeRow{{"x": 1}, {"w": 1}, {"z": 1}}

	a := matchThemes(prev, newRows, 1)
	b := matchThemes(prev, newRows, 1)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("matchThemes is not deterministic:\n a=%+v\n b=%+v", a, b)
	}
	// theme-1 inherited; two fresh mints from counter 1 → theme-2, theme-3.
	if !reflect.DeepEqual(a.minted, []string{"theme-2", "theme-3"}) {
		t.Fatalf("minted = %v, want [theme-2 theme-3] (never reuse)", a.minted)
	}
}

// sqrtPos is a tiny local helper so the boundary test can build a row with an
// exact target cosine without importing math into the test file.
func sqrtPos(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Newton's method, plenty accurate for the test's purposes.
	g := x
	for range 40 {
		g = 0.5 * (g + x/g)
	}
	return g
}

// --- Cache-level identity tests ---

// mutableCache wires a themes cache over mutable store/tag/revision stubs so a
// test can evolve the corpus across revisions and hold references to mutate.
func mutableCache(items []issues.Issue, tags []issues.Tag) (*Cache, *stubStore, *stubTags, *stubRevisions) {
	store := &stubStore{items: items}
	tagSrc := &stubTags{tags: tags}
	rev := &stubRevisions{}
	rev.rev.Store(1)
	lambda := &ridgelambda.Cache{Store: store, Tags: tagSrc, Revisions: rev}
	decomp := &ridgedecomp.Cache{Store: store, Tags: tagSrc, Revisions: rev, Lambda: lambda}
	return &Cache{Decomp: decomp, Store: store, Revisions: rev}, store, tagSrc, rev
}

// TestCacheRetainsIdentityAcrossRevisionBump asserts the previous result is
// retained across a recompute: an unchanged corpus at a bumped revision keeps
// every stable ID and mints/retires nothing.
func TestCacheRetainsIdentityAcrossRevisionBump(t *testing.T) {
	c, _, _, rev := mutableCache(clusteredIssues(24), themeTags())

	first, ok, err := c.Current(context.Background())
	if err != nil || !ok {
		t.Fatalf("first compute: ok=%v err=%v", ok, err)
	}
	if len(first.Identities) != len(first.Themes) {
		t.Fatalf("Identities (%d) must be index-aligned with Themes (%d)", len(first.Identities), len(first.Themes))
	}
	if len(first.MintedIDs) != len(first.Themes) {
		t.Fatalf("first compute should mint every theme, got %d mints for %d themes", len(first.MintedIDs), len(first.Themes))
	}
	firstIDs := stableIDs(first)

	// Bump the revision without changing the corpus → recompute, same themes.
	rev.rev.Add(1)
	second, ok, err := c.Current(context.Background())
	if err != nil || !ok {
		t.Fatalf("second compute: ok=%v err=%v", ok, err)
	}
	if c.computeCount != 2 {
		t.Fatalf("expected a recompute after the revision bump, computeCount=%d", c.computeCount)
	}
	if got := stableIDs(second); !reflect.DeepEqual(got, firstIDs) {
		t.Fatalf("stable IDs churned across a no-op recompute: %v -> %v", firstIDs, got)
	}
	if len(second.MintedIDs) != 0 || len(second.RetiredIDs) != 0 {
		t.Fatalf("retained identity must not mint/retire on an unchanged corpus; minted=%v retired=%v", second.MintedIDs, second.RetiredIDs)
	}
	for i, id := range second.Identities {
		if id.MatchScore < 0.98 {
			t.Fatalf("theme %d (%s) rematched at %.4f, expected ~1.0", i, id.StableID, id.MatchScore)
		}
	}
}

// TestCacheRestartMintsFreshIDs asserts the documented cold-start behavior: a new
// Cache instance (a process restart) has no retained identity, so its first
// compute mints a fresh ID for every theme rather than inheriting.
func TestCacheRestartMintsFreshIDs(t *testing.T) {
	items := clusteredIssues(24)
	tags := themeTags()

	a, _, _, _ := mutableCache(items, tags)
	if _, ok, err := a.Current(context.Background()); err != nil || !ok {
		t.Fatalf("cache A compute: ok=%v err=%v", ok, err)
	}

	// A brand-new Cache over the same corpus starts with a zero counter and no
	// previous result: every theme mints.
	b, _, _, _ := mutableCache(items, tags)
	res, ok, err := b.Current(context.Background())
	if err != nil || !ok {
		t.Fatalf("cache B compute: ok=%v err=%v", ok, err)
	}
	if len(res.MintedIDs) != len(res.Themes) {
		t.Fatalf("restart should mint every theme, got %d mints for %d themes", len(res.MintedIDs), len(res.Themes))
	}
	if len(res.RetiredIDs) != 0 {
		t.Fatalf("restart has no predecessors to retire, got %v", res.RetiredIDs)
	}
	minted := make(map[string]bool, len(res.MintedIDs))
	for _, id := range res.MintedIDs {
		minted[id] = true
	}
	for _, id := range res.Identities {
		if !minted[id.StableID] {
			t.Fatalf("theme %s is not in MintedIDs, so it was inherited across a restart", id.StableID)
		}
		if id.MatchScore != 0 {
			t.Fatalf("minted theme %s must have a zero match score, got %.4f", id.StableID, id.MatchScore)
		}
	}
}

// --- Soak test: the WP's acceptance test ---

// TestSoakIdentityStability drives a fixture corpus through a sequence of small
// mutations across >=10 refreshes and asserts theme identity is rock-steady —
// stable IDs persist with no spurious mint/retire churn — then a single
// deliberate structural change (deleting a whole theme's tag and its issues)
// produces exactly the expected retire and nothing else.
func TestSoakIdentityStability(t *testing.T) {
	const groups, perGroup = 6, 8
	c, store, tagSrc, rev := mutableCache(groupedIssues(groups, perGroup), groupTags(groups))

	res, ok, err := c.Current(context.Background())
	if err != nil || !ok {
		t.Fatalf("initial compute: ok=%v err=%v", ok, err)
	}
	if len(res.Themes) != groups {
		t.Fatalf("expected %d cleanly-separated themes, got %d", groups, len(res.Themes))
	}
	if len(res.MintedIDs) != groups || len(res.RetiredIDs) != 0 {
		t.Fatalf("first refresh mints all, retires none; minted=%d retired=%v", len(res.MintedIDs), res.RetiredIDs)
	}

	// Baseline map: each theme's dominant tag -> stable ID. Clean separation
	// makes the top tag a durable label for "which theme this is".
	baseline := topTagToID(t, res)
	if len(baseline) != groups {
		t.Fatalf("expected %d distinct dominant tags, got %d (themes are not cleanly separated)", groups, len(baseline))
	}

	// >=10 refreshes of small, deterministic mutations. Each mutation touches one
	// group without emptying it, so K and the theme structure are unchanged.
	counter := perGroup // per-group running id suffix for added issues
	for step := 0; step < 12; step++ {
		grp := step % groups
		switch step % 3 {
		case 0: // add an issue to a group
			store.items = append(store.items, groupIssue(grp, counter, groups))
			counter++
		case 1: // tweak a loading on an existing issue of the group
			tweakGroupIssue(store, grp)
		case 2: // remove an issue from a group (it still has plenty)
			removeOneFromGroup(store, grp)
		}
		rev.rev.Add(1)

		got, ok, err := c.Current(context.Background())
		if err != nil || !ok {
			t.Fatalf("refresh %d: ok=%v err=%v", step, ok, err)
		}
		if len(got.MintedIDs) != 0 || len(got.RetiredIDs) != 0 {
			t.Fatalf("refresh %d churned identity: minted=%v retired=%v", step, got.MintedIDs, got.RetiredIDs)
		}
		cur := topTagToID(t, got)
		if !reflect.DeepEqual(cur, baseline) {
			t.Fatalf("refresh %d: dominant-tag -> ID map drifted\n base=%v\n got =%v", step, baseline, cur)
		}
	}

	// The deliberate structural change: delete an entire theme — remove group 5's
	// tag from the catalog AND all its issues. Exactly that theme must retire; the
	// other five keep their IDs and nothing mints.
	victimTag := fmt.Sprintf("grp-%d", groups-1)
	victimID := baseline[victimTag]
	removeGroup(store, tagSrc, groups-1)
	rev.rev.Add(1)

	after, ok, err := c.Current(context.Background())
	if err != nil || !ok {
		t.Fatalf("post-structural compute: ok=%v err=%v", ok, err)
	}
	if len(after.Themes) != groups-1 {
		t.Fatalf("expected %d themes after deleting one, got %d", groups-1, len(after.Themes))
	}
	if !reflect.DeepEqual(after.RetiredIDs, []string{victimID}) {
		t.Fatalf("expected exactly [%s] retired, got %v", victimID, after.RetiredIDs)
	}
	if len(after.MintedIDs) != 0 {
		t.Fatalf("structural deletion should not mint, got %v", after.MintedIDs)
	}
	survivors := topTagToID(t, after)
	for tag, id := range baseline {
		if tag == victimTag {
			continue
		}
		if survivors[tag] != id {
			t.Fatalf("survivor theme %s changed ID after deletion: %s -> %s", tag, id, survivors[tag])
		}
	}
}

// stableIDs returns the stable ID of each theme in Themes order.
func stableIDs(r Result) []string {
	out := make([]string, len(r.Identities))
	for i, id := range r.Identities {
		out[i] = id.StableID
	}
	return out
}

// topTagToID maps each theme's dominant (top) tag to its stable ID, asserting
// every theme has a top tag and the mapping is one-to-one.
func topTagToID(t *testing.T, r Result) map[string]string {
	t.Helper()
	out := make(map[string]string, len(r.Themes))
	for i, theme := range r.Themes {
		if len(theme.Tags) == 0 {
			t.Fatalf("theme %d (%s) has no tags", i, r.Identities[i].StableID)
		}
		top := theme.Tags[0].Tag
		if _, dup := out[top]; dup {
			t.Fatalf("two themes share dominant tag %q — not cleanly separated", top)
		}
		out[top] = r.Identities[i].StableID
	}
	return out
}

// --- Fixture builders for the soak test ---

// axisEmbedding returns a unit vector along axis idx in dim dimensions, with a
// small deterministic off-axis jitter so issues in a group are distinct without
// leaving the cluster.
func axisEmbedding(idx, dim int, jitter float64) []float64 {
	if dim < 4 {
		dim = 4
	}
	v := make([]float64, dim)
	v[idx] = 1.0
	v[(idx+1)%dim] += jitter
	return unit(v)
}

// groupTags returns g tags on mutually orthogonal embedding axes, so each group's
// issues load cleanly onto their own tag and NMF separates them into distinct
// themes.
func groupTags(g int) []issues.Tag {
	dim := g
	if dim < 4 {
		dim = 4
	}
	tags := make([]issues.Tag, g)
	for i := 0; i < g; i++ {
		tags[i] = issues.Tag{Name: fmt.Sprintf("grp-%d", i), Embedding: axisEmbedding(i, dim, 0)}
	}
	return tags
}

// groupIssue builds one issue anchored to group grp with an embedding near that
// group's axis; k is the intra-group index used for deterministic jitter and ID.
func groupIssue(grp, k, groups int) issues.Issue {
	jitter := float64(k%5) * 0.01
	return issues.Issue{
		ID:        fmt.Sprintf("g%02d-i%03d", grp, k),
		Status:    issues.StatusOpen,
		TagScores: []issues.TagRelevance{{Tag: fmt.Sprintf("grp-%d", grp), Relevance: 0.8 + jitter}},
		Embedding: axisEmbedding(grp, groups, jitter),
	}
}

// groupedIssues builds a corpus of g groups, perGroup issues each, cleanly
// separated so the factorization yields one theme per group.
func groupedIssues(g, perGroup int) []issues.Issue {
	items := make([]issues.Issue, 0, g*perGroup)
	for grp := 0; grp < g; grp++ {
		for k := 0; k < perGroup; k++ {
			items = append(items, groupIssue(grp, k, g))
		}
	}
	return items
}

// tweakGroupIssue nudges the loading of the first issue found in group grp — a
// small mutation that should not move theme identity.
func tweakGroupIssue(store *stubStore, grp int) {
	prefix := fmt.Sprintf("g%02d-", grp)
	for i := range store.items {
		if hasPrefix(store.items[i].ID, prefix) {
			if len(store.items[i].TagScores) > 0 {
				store.items[i].TagScores[0].Relevance = 0.95
			}
			store.items[i].Embedding = axisEmbedding(grp, groupsOf(store), 0.03)
			return
		}
	}
}

// removeOneFromGroup drops the last issue in group grp (the group keeps plenty).
func removeOneFromGroup(store *stubStore, grp int) {
	prefix := fmt.Sprintf("g%02d-", grp)
	for i := len(store.items) - 1; i >= 0; i-- {
		if hasPrefix(store.items[i].ID, prefix) {
			store.items = append(store.items[:i], store.items[i+1:]...)
			return
		}
	}
}

// removeGroup deletes an entire theme: every issue in group grp plus its tag from
// the catalog. This is the deliberate structural change the soak test asserts on.
func removeGroup(store *stubStore, tagSrc *stubTags, grp int) {
	prefix := fmt.Sprintf("g%02d-", grp)
	kept := store.items[:0:0]
	for _, it := range store.items {
		if !hasPrefix(it.ID, prefix) {
			kept = append(kept, it)
		}
	}
	store.items = kept

	name := fmt.Sprintf("grp-%d", grp)
	keptTags := tagSrc.tags[:0:0]
	for _, tg := range tagSrc.tags {
		if tg.Name != name {
			keptTags = append(keptTags, tg)
		}
	}
	tagSrc.tags = keptTags
}

// groupsOf counts the distinct groups currently in the store, so tweak rebuilds
// an embedding in the right dimensionality regardless of how many groups remain.
func groupsOf(store *stubStore) int {
	seen := make(map[string]bool)
	for _, it := range store.items {
		if len(it.ID) >= 3 {
			seen[it.ID[:3]] = true
		}
	}
	return len(seen)
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
