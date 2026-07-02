package themes

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"sortit/internal/issuethemes"
)

// themeMatchThreshold is the minimum cosine (over aligned tag loadings) at which
// a new theme inherits a previous theme's stable ID. NMF has no identifiability
// guarantee, so a refresh can permute, split, or merge components; matching the
// new H rows to the previous ones by similarity is what keeps "theme 3" the same
// theme across page loads.
//
// Below this cosine the "match" is a different theme wearing the same assignment
// slot — inheriting the ID there would silently swap identities, the exact
// failure this WP exists to prevent — so a below-threshold pair mints a fresh ID
// and retires the old one instead of pretending continuity. 0.6 is a deliberate
// starting point (a theme keeps its identity through routine drift but not
// through a structural change); revisit once WP-206 telemetry shows the observed
// same-theme vs different-theme cosine distributions on the real corpus.
const themeMatchThreshold = 0.6

// ThemeIdentity carries the WP-203 stable-identity metadata for the theme at the
// same index in Result.Themes. It is a parallel slice rather than a field on
// issuethemes.Theme because that pure library type stays caller-free and cannot
// carry service-layer state; the wrapper owns identity, the library owns the
// factorization.
type ThemeIdentity struct {
	// StableID is the theme's identity across refreshes, formatted "theme-<n>".
	// It is decoupled from the NMF component index and from display order: a
	// theme keeps its ID as long as its H row stays similar enough to match,
	// regardless of where the factorization or the W-mass ordering places it.
	StableID string
	// MatchScore is the cosine to the predecessor theme whose ID this theme
	// inherited. It is 0 for a minted theme (no predecessor) and feeds the
	// WP-206 stability telemetry / WP-204 API. A value at or just above
	// themeMatchThreshold flags a shaky match a human may want to look at.
	MatchScore float64
}

// themeRow is a theme's tag-loading vector keyed by tag NAME, the matcher's unit
// of comparison. Keying by name (not column index) is deliberate: the tag
// catalog can change between revisions (tags added or removed), so two revisions
// need not share a column layout. Cosine over the union of names — a tag absent
// from one side contributing 0 — aligns the rows correctly regardless.
//
// The row is built from issuethemes.Theme.Tags, which is the top-K slice of the
// L1-normalized H row (K = the factorizer's TopTags, default 5). Cosine is
// scale-invariant, so the missing L1 mass in the tail does not matter; matching
// on the dominant tags is a faithful proxy for matching on the full H row as
// long as a theme's identity lives in its top tags, which it does by
// construction (themes are defined by their heaviest loadings). If WP-206
// telemetry ever shows churn traceable to tail truncation, the fix is to raise
// the factorizer's TopTags (or expose the full H row) — not to touch this
// matcher, which already handles the full-row case unchanged.
type themeRow map[string]float64

// themeRowFrom projects a factorized theme onto its name-keyed loading row,
// dropping non-positive loadings (they carry no similarity signal).
func themeRowFrom(t issuethemes.Theme) themeRow {
	row := make(themeRow, len(t.Tags))
	for _, tag := range t.Tags {
		if tag.Loading > 0 {
			row[tag.Tag] = tag.Loading
		}
	}
	return row
}

// cosine is the cosine similarity between two name-keyed loading rows over the
// union of their tags (a tag present on only one side contributes 0 to the dot
// product but still to that side's norm). Empty or zero-norm rows are treated as
// orthogonal (0), never matched.
func cosine(a, b themeRow) float64 {
	var dot, na, nb float64
	for tag, av := range a {
		na += av * av
		if bv, ok := b[tag]; ok {
			dot += av * bv
		}
	}
	for _, bv := range b {
		nb += bv * bv
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// identifiedTheme is one previous-revision theme retained as cross-refresh
// state: its stable ID and the H row it carried, so the next revision's themes
// can be matched against it.
type identifiedTheme struct {
	id  string
	row themeRow
}

// identityState is the cache's cross-revision identity memory: the previous
// revision's identified themes plus the monotonic counter that mints new IDs and
// never reuses one. It is in-memory only, so a process restart resets identity —
// acceptable at the debug tier (WP-204), documented on the Cache. Persisting
// this tiny state (ID → H row, ~K×T floats) is the SECOND trigger for theme
// persistence (after WP-405's drift snapshots): it becomes necessary when
// Stage 4 ships user-visible profiles, where an ID silently changing on restart
// is a visible regression. Guarded by Cache.mu (identity is mutated only inside
// the mutex-held compute).
type identityState struct {
	prev    []identifiedTheme
	counter int
}

// matchResult is the outcome of matching a revision's new themes to the previous
// ones: a stable ID and match score per new theme (index-aligned with the input
// rows), the IDs minted this revision, the previous IDs that retired, and the
// advanced counter.
type matchResult struct {
	ids     []string
	scores  []float64
	minted  []string
	retired []string
	counter int
}

// matchThemes assigns stable IDs to newRows given the previous identified themes
// and the current mint counter. It is a pure function: identical inputs
// (including counter and the order of prev) produce identical output — no RNG,
// no dependence on map iteration order. Ties in the optimal assignment are broken
// deterministically toward the lower stable ID then the lower theme index (prev
// is matched in ascending numeric-ID order and rows are scanned in index order,
// and the Hungarian solver prefers lower indices on equal cost).
//
// Similarity is cosine between L1-normalized H rows; the optimal one-to-one
// assignment is found with the Hungarian algorithm (K ≤ ~10, so O(K³) is
// trivial and removes any doubt a greedy pass would leave). A matched pair
// inherits the previous ID only when its cosine is at least themeMatchThreshold;
// otherwise the new theme mints a fresh ID and, if it displaced a real
// predecessor in the assignment, that predecessor is left to retire. Previous
// IDs matched by no qualifying new theme are reported as retired.
func matchThemes(prev []identifiedTheme, newRows []themeRow, counter int) matchResult {
	kNew := len(newRows)

	// Match against prev in ascending numeric-ID order so the assignment's
	// tie-breaking is deterministic and favors older (lower-numbered) IDs.
	ordered := append([]identifiedTheme(nil), prev...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return themeIDNum(ordered[i].id) < themeIDNum(ordered[j].id)
	})
	kOld := len(ordered)

	res := matchResult{
		ids:     make([]string, kNew),
		scores:  make([]float64, kNew),
		counter: counter,
	}
	if kNew == 0 {
		// Every previous theme retires when a revision produces no themes.
		for _, p := range ordered {
			res.retired = append(res.retired, p.id)
		}
		return res
	}

	// Square cost matrix padded to n×n: rows are new themes, columns are
	// previous themes. cost = -cosine so the min-cost assignment maximizes total
	// similarity; padding cells are 0 (worse than any real, positive-cosine
	// match), so real predecessors are matched whenever it helps.
	n := max(kNew, kOld)
	cost := make([][]float64, n)
	for i := range cost {
		cost[i] = make([]float64, n)
	}
	// Real cells: row i (a new theme) vs column j (a previous theme). Padding
	// cells stay 0. Ranging over the source slices keeps the indexing provably
	// in range (kNew, kOld ≤ n).
	for i, row := range newRows {
		for j, prevTheme := range ordered {
			cost[i][j] = -cosine(row, prevTheme.row)
		}
	}
	assign := hungarian(cost)

	prevMatched := make([]bool, kOld)
	for i := 0; i < kNew; i++ {
		j := assign[i]
		if j < kOld {
			sim := -cost[i][j]
			if sim >= themeMatchThreshold {
				res.ids[i] = ordered[j].id
				res.scores[i] = sim
				prevMatched[j] = true
				continue
			}
		}
		res.counter++
		id := fmt.Sprintf("theme-%d", res.counter)
		res.ids[i] = id
		res.minted = append(res.minted, id)
	}
	for j := 0; j < kOld; j++ {
		if !prevMatched[j] {
			res.retired = append(res.retired, ordered[j].id)
		}
	}
	return res
}

// themeIDNum extracts n from a "theme-<n>" stable ID for ordering. A malformed
// ID sorts first (returns -1); it never occurs in practice since IDs are minted
// only by matchThemes.
func themeIDNum(id string) int {
	s := strings.TrimPrefix(id, "theme-")
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return n
}

// hungarian solves the minimum-cost assignment for a square cost matrix in
// O(n³) using the potentials (Jonker–Volgenant style) formulation. It returns
// assign where assign[i] is the column assigned to row i. It is fully
// deterministic: for fixed input it scans columns in ascending order and uses
// strict comparisons, so equal-cost ties resolve toward the lower column index.
// This is a self-contained implementation — no external dependency, as the plan
// requires — and it is exercised exhaustively by the matcher tests.
func hungarian(cost [][]float64) []int {
	n := len(cost)
	assign := make([]int, n)
	if n == 0 {
		return assign
	}
	const inf = math.MaxFloat64

	u := make([]float64, n+1) // row potentials
	v := make([]float64, n+1) // column potentials
	p := make([]int, n+1)     // p[j] = row currently assigned to column j (0 = none)
	way := make([]int, n+1)   // predecessor column in the augmenting path

	for i := 1; i <= n; i++ {
		p[0] = i
		j0 := 0
		minv := make([]float64, n+1)
		used := make([]bool, n+1)
		for j := range minv {
			minv[j] = inf
		}
		for {
			used[j0] = true
			i0 := p[j0]
			delta := inf
			j1 := -1
			for j := 1; j <= n; j++ {
				if used[j] {
					continue
				}
				cur := cost[i0-1][j-1] - u[i0] - v[j]
				if cur < minv[j] {
					minv[j] = cur
					way[j] = j0
				}
				if minv[j] < delta {
					delta = minv[j]
					j1 = j
				}
			}
			for j := 0; j <= n; j++ {
				if used[j] {
					u[p[j]] += delta
					v[j] -= delta
				} else {
					minv[j] -= delta
				}
			}
			j0 = j1
			if p[j0] == 0 {
				break
			}
		}
		for j0 != 0 {
			j1 := way[j0]
			p[j0] = p[j1]
			j0 = j1
		}
	}
	for j := 1; j <= n; j++ {
		if p[j] != 0 {
			assign[p[j]-1] = j - 1
		}
	}
	return assign
}
