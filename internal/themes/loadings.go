package themes

import (
	"sort"

	"sortit/internal/domain"
	"sortit/internal/issuemath"
	"sortit/internal/issues"
	"sortit/internal/issuethemes"
)

// participationCounts records how the corpus split under the theme
// participation rule. participating issues feed the NMF; the excluded tallies
// are surfaced by the debug API (WP-204) so a shrinking participant set is
// observable rather than silent.
type participationCounts struct {
	participating int
	noEmbedding   int
	noAnchor      int
}

// loadingsFromDecomposition maps a cached full-corpus ridge decomposition into
// the []issuethemes.IssueLoading NMF input, applying the theme participation
// rule and reporting the corpus split.
//
// Participation rule (matches the drift sweep's rule in
// internal/issuemath/ridge_drift.go): an issue participates iff it has a usable
// embedding AND at least one anchored tag. "Usable embedding" is read from the
// decomposition itself — ComputeRidgeDecomposition only admits issues with a
// non-zero, correctly-shaped embedding, so VectorsFor(id) returning ok is
// exactly the usable-embedding gate. "Anchored tag" means the analyzer scored
// (or negated) at least one catalog tag; issues with no analyzer opinion carry
// a loading shaped purely by the unscored penalty (near-zero at the GCV λ) that
// NMF drops anyway, so they are excluded explicitly for clarity rather than
// left to be dropped implicitly.
//
// Closed issues are NOT filtered: themes describe the corpus, not the backlog
// (Stage 4 overlays filter by lifecycle at query time), so a closed issue with
// an embedding and an anchor participates like any other.
//
// The loading values are the RAW ridge loadings f = Loading·LoadingNorm, not
// the unit Loading the ranker compares. The NMF input is V = max(0, f): an
// issue with a stronger tag geometry must contribute more mass, which unit
// loadings would flatten. RidgeVectors.LoadingNorm carries the pre-normalization
// magnitude precisely so the raw f is recoverable from the cached vectors
// without redoing any solve (see internal/issuemath/ridge_decomposition.go). The
// tag order is the decomposition's TagNames, so f[j] aligns with TagNames[j];
// issuethemes.Build's own positiveMatrix takes the max(0, ·) and drops
// all-nonpositive rows.
//
// Iteration is over an ID-sorted copy of items so the output is deterministic
// regardless of the store's row order (conventions §4).
func loadingsFromDecomposition(
	decomp *issuemath.CorpusRidgeDecomposition,
	items []issues.Issue,
) ([]issuethemes.IssueLoading, participationCounts) {
	width := len(decomp.TagNames)
	tagIndex := make(map[string]int, width)
	for i, tag := range decomp.TagNames {
		tagIndex[tag] = i
	}

	sorted := append([]issues.Issue(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	loadings := make([]issuethemes.IssueLoading, 0, len(sorted))
	var counts participationCounts
	for _, item := range sorted {
		vecs, ok := decomp.Decomposition.VectorsFor(item.ID)
		if !ok {
			counts.noEmbedding++
			continue
		}
		if !anchoredAny(item.TagScores, tagIndex) {
			counts.noAnchor++
			continue
		}
		loadings = append(loadings, issuethemes.IssueLoading{
			IssueID: item.ID,
			Values:  rawLoading(vecs, width),
		})
		counts.participating++
	}
	return loadings, counts
}

// ParticipationStatus classifies why a single issue does or does not feed the
// theme factorization, mirroring the corpus-wide rule loadingsFromDecomposition
// applies. Exported for the debug API (WP-204's /debug/issues/{id}/themes),
// which needs the classification for one issue on request without re-running
// the whole-corpus adapter or duplicating the rule.
type ParticipationStatus string

const (
	// ParticipationParticipating means the issue cleared both gates and fed
	// the NMF.
	ParticipationParticipating ParticipationStatus = "participating"
	// ParticipationExcludedNoEmbedding means the decomposition has no usable
	// vectors for this issue (no embedding, or one the decomposition rejected).
	ParticipationExcludedNoEmbedding ParticipationStatus = "excluded-no-embedding"
	// ParticipationExcludedNoAnchor means the issue has a usable embedding but
	// no analyzer opinion (scored or negated) on any catalog tag.
	ParticipationExcludedNoAnchor ParticipationStatus = "excluded-no-anchor"
)

// ClassifyParticipation reports the participation status of one issue against
// a corpus ridge decomposition, applying the exact rule
// loadingsFromDecomposition applies corpus-wide. A nil decomposition (no
// decomposition available at all) reports ParticipationExcludedNoEmbedding —
// there is nothing to have decomposed the issue into.
func ClassifyParticipation(decomp *issuemath.CorpusRidgeDecomposition, issue issues.Issue) ParticipationStatus {
	if decomp == nil {
		return ParticipationExcludedNoEmbedding
	}
	if _, ok := decomp.Decomposition.VectorsFor(issue.ID); !ok {
		return ParticipationExcludedNoEmbedding
	}
	tagIndex := make(map[string]int, len(decomp.TagNames))
	for i, tag := range decomp.TagNames {
		tagIndex[tag] = i
	}
	if !anchoredAny(issue.TagScores, tagIndex) {
		return ParticipationExcludedNoAnchor
	}
	return ParticipationParticipating
}

// anchoredAny reports whether the analyzer expressed an opinion on any catalog
// tag — the same "anchoredAny" notion as ComputeCorpusDrift. Presence in the
// tag index is the test, matching signedAnchor's `scored` mask (a zero-relevance
// score still counts as an opinion).
func anchoredAny(tagScores []domain.TagRelevance, tagIndex map[string]int) bool {
	for _, ts := range tagScores {
		if _, ok := tagIndex[ts.Tag]; ok {
			return true
		}
	}
	return false
}

// rawLoading reconstructs the raw ridge loading f = Loading·LoadingNorm as a
// width-length row aligned with the decomposition's TagNames. A residual-only
// issue (nil Loading, zero LoadingNorm) or a shape mismatch yields an all-zero
// row, which issuethemes.Build drops as all-nonpositive — such an issue still
// counts as participating (it cleared both gates) but contributes no theme mass.
func rawLoading(v issuemath.RidgeVectors, width int) []float64 {
	out := make([]float64, width)
	if len(v.Loading) != width || v.LoadingNorm == 0 {
		return out
	}
	for i, u := range v.Loading {
		out[i] = u * v.LoadingNorm
	}
	return out
}
