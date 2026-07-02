package diagnostics

import (
	"cmp"
	"context"
	"errors"
	"slices"

	"sortit/internal/issuemath"
	"sortit/internal/issues"
	"sortit/internal/issuethemes"
	"sortit/internal/ridgedecomp"
	"sortit/internal/themes"
	"sortit/internal/vectors"
)

// ErrThemeNotFound is returned by DebugThemeDetailHandler when the requested
// stable theme ID does not match any theme in the current revision's result —
// mapped to 404 at the API layer, distinct from the (zero, false) degraded
// shape returned when the theme cache itself has nothing computed.
var ErrThemeNotFound = errors.New("theme not found")

// debugThemeTopIssues caps the centroid-nearest and top-by-weight issue lists
// on the theme detail endpoint, matching the "top ~5" the WP asks for.
const debugThemeTopIssues = 5

// DebugThemeTag is one tag loading within a theme, surfaced verbatim from
// issuethemes.Theme.Tags (the factorizer's top-5 H-row entries — see
// DebugThemeDetailResult.TagsNote).
type DebugThemeTag struct {
	Tag     string  `json:"tag"`
	Loading float64 `json:"loading"`
}

// DebugThemeSummary is one theme's entry in the /debug/themes list, and the
// shared header embedded in the /debug/themes/{id} detail response.
type DebugThemeSummary struct {
	// StableID is the WP-203 cross-refresh identity, not the NMF component
	// index or display position.
	StableID string `json:"stableId"`
	// Label is the theme's human name (WP-205): an LLM name when the labeler has
	// produced one, otherwise the deterministic top-tag fallback (e.g.
	// "billing / payments"). Every theme always carries one.
	Label string `json:"label,omitempty"`
	// Description is the theme's one-sentence summary; empty until the LLM label
	// lands (the fallback carries no description).
	Description string `json:"description,omitempty"`
	// PreviousLabel is the name this theme carried before its most recent
	// relabel, surfaced so a consumer can show continuity; omitted when the
	// theme has never relabeled.
	PreviousLabel string `json:"previousLabel,omitempty"`
	// Weight is this theme's share of total W mass across all themes this
	// revision (sums to ~1 across the list response).
	Weight float64         `json:"weight"`
	Tags   []DebugThemeTag `json:"tags"`
	// MatchScore is the cosine to the predecessor theme this ID was inherited
	// from; 0 for a theme minted this revision.
	MatchScore float64 `json:"matchScore"`
	// Minted reports whether this theme's stable ID was first assigned this
	// revision (no qualifying predecessor).
	Minted bool `json:"minted"`
}

// DebugThemesListResult is the response body for GET /api/v1/debug/themes.
// Computed is false — with every other field zero — when the theme cache
// currently has nothing usable (below the participation floor, or the
// decomposition it depends on is unavailable); mirrors
// DebugTagHealthResult's "not computed" shape.
type DebugThemesListResult struct {
	Computed            bool     `json:"computed"`
	Revision            uint64   `json:"revision"`
	Participating       int      `json:"participating"`
	ExcludedNoEmbedding int      `json:"excludedNoEmbedding"`
	ExcludedNoAnchor    int      `json:"excludedNoAnchor"`
	MintedIDs           []string `json:"mintedIds,omitempty"`
	RetiredIDs          []string `json:"retiredIds,omitempty"`
	// Themes is sorted by Weight descending, StableID tiebreak.
	Themes []DebugThemeSummary `json:"themes"`
}

// DebugThemeIssueRef is one issue surfaced against a theme. Score's meaning
// depends on which list it appears in (see DebugThemeDetailResult's field
// comments) — it is never comparable across the two lists.
type DebugThemeIssueRef struct {
	ID    string  `json:"id"`
	Raw   string  `json:"raw"`
	Score float64 `json:"score"`
}

// DebugThemeDetailResult is the response body for GET /api/v1/debug/themes/{id}.
// Computed is false when the theme cache has nothing usable this revision (the
// requested ID could not have existed either way); an ID that genuinely does
// not match any current theme is a 404 (ErrThemeNotFound), not this shape.
type DebugThemeDetailResult struct {
	Computed bool `json:"computed"`
	DebugThemeSummary
	// TagsNote documents a known limitation: Tags above is issuethemes.Theme's
	// top-5 H-row loadings, not the full row — issuethemes does not export the
	// full row (WP-203's identity matcher has the same limitation; see
	// internal/themes/identity.go's themeRow doc). A theme with meaningful mass
	// past its top 5 tags will look narrower here than the factorization
	// actually holds.
	TagsNote string `json:"tagsNote,omitempty"`
	// CentroidTopIssues are the issues whose centered embedding is most
	// cosine-similar to this theme's centroid, independent of whether the
	// issue fed the NMF — an excluded issue can still surface here as "this is
	// what the theme's semantic direction looks like in the corpus". Score is
	// the cosine similarity in [-1, 1].
	CentroidTopIssues []DebugThemeIssueRef `json:"centroidTopIssues,omitempty"`
	// TopIssuesByWeight are the participating issues with the largest raw W
	// membership weight in this theme. Score is the raw (unnormalized) W
	// entry — comparable within this list only, not against Weight above or
	// against another theme's list.
	TopIssuesByWeight []DebugThemeIssueRef `json:"topIssuesByWeight,omitempty"`
}

// DebugThemeWeight pairs a stable theme ID with an issue's raw W membership
// weight in that theme.
type DebugThemeWeight struct {
	StableID string  `json:"stableId"`
	Weight   float64 `json:"weight"`
}

// DebugIssueThemesResult is the response body for
// GET /api/v1/debug/issues/{id}/themes. Participation is populated whenever a
// ridge decomposition is available, independent of Computed — knowing *why*
// an issue is or isn't in the theme list is useful even when the theme cache
// itself is degraded (e.g. a corpus below the theme participation floor still
// has a usable decomposition).
type DebugIssueThemesResult struct {
	IssueID  string `json:"issueId"`
	Computed bool   `json:"computed"`
	// Participation is one of themes.ParticipationStatus's values, or empty
	// when no decomposition was available to classify against.
	Participation string `json:"participation,omitempty"`
	// Themes is this issue's full theme-membership row (all K themes, one
	// entry per theme in the current result), sorted by Weight descending.
	Themes []DebugThemeWeight `json:"themes,omitempty"`
}

// DebugThemesListHandler serves GET /api/v1/debug/themes.
type DebugThemesListHandler struct {
	Cache *themes.Cache
}

// Handle returns the current theme list, sorted by weight share descending.
func (h DebugThemesListHandler) Handle(ctx context.Context) (DebugThemesListResult, error) {
	if h.Cache == nil {
		return DebugThemesListResult{}, nil
	}
	res, ok, err := h.Cache.Current(ctx)
	if err != nil {
		return DebugThemesListResult{}, err
	}
	if !ok {
		return DebugThemesListResult{}, nil
	}

	minted := make(map[string]bool, len(res.MintedIDs))
	for _, id := range res.MintedIDs {
		minted[id] = true
	}
	totalWeight := totalThemeWeight(res.Themes)

	summaries := make([]DebugThemeSummary, len(res.Themes))
	for i, theme := range res.Themes {
		identity := identityAt(res.Identities, i)
		label := labelAt(res.Labels, i)
		summaries[i] = DebugThemeSummary{
			StableID:      identity.StableID,
			Label:         label.Label,
			Description:   label.Description,
			PreviousLabel: label.PreviousLabel,
			Weight:        round3(themeWeightShare(theme.Weight, totalWeight)),
			Tags:          toDebugThemeTags(theme.Tags),
			MatchScore:    round3(identity.MatchScore),
			Minted:        minted[identity.StableID],
		}
	}
	sortDebugThemeSummaries(summaries)

	return DebugThemesListResult{
		Computed:            true,
		Revision:            res.Revision,
		Participating:       res.Participating,
		ExcludedNoEmbedding: res.ExcludedNoEmbedding,
		ExcludedNoAnchor:    res.ExcludedNoAnchor,
		MintedIDs:           res.MintedIDs,
		RetiredIDs:          res.RetiredIDs,
		Themes:              summaries,
	}, nil
}

// DebugThemeDetailHandler serves GET /api/v1/debug/themes/{id}.
type DebugThemeDetailHandler struct {
	Cache *themes.Cache
	Store issues.Store
}

// Handle returns one theme's detail by its WP-203 stable ID.
func (h DebugThemeDetailHandler) Handle(ctx context.Context, stableID string) (DebugThemeDetailResult, error) {
	if h.Cache == nil {
		return DebugThemeDetailResult{}, nil
	}
	res, ok, err := h.Cache.Current(ctx)
	if err != nil {
		return DebugThemeDetailResult{}, err
	}
	if !ok {
		return DebugThemeDetailResult{}, nil
	}

	index := -1
	for i, identity := range res.Identities {
		if identity.StableID == stableID {
			index = i
			break
		}
	}
	if index < 0 || index >= len(res.Themes) {
		return DebugThemeDetailResult{}, ErrThemeNotFound
	}
	theme := res.Themes[index]
	identity := res.Identities[index]
	label := labelAt(res.Labels, index)

	minted := false
	for _, id := range res.MintedIDs {
		if id == stableID {
			minted = true
			break
		}
	}

	var storeIssues []issues.Issue
	if h.Store != nil {
		items, err := h.Store.List(ctx)
		if err != nil {
			return DebugThemeDetailResult{}, err
		}
		storeIssues = items
	}

	return DebugThemeDetailResult{
		Computed: true,
		DebugThemeSummary: DebugThemeSummary{
			StableID:      identity.StableID,
			Label:         label.Label,
			Description:   label.Description,
			PreviousLabel: label.PreviousLabel,
			Weight:        round3(themeWeightShare(theme.Weight, totalThemeWeight(res.Themes))),
			Tags:          toDebugThemeTags(theme.Tags),
			MatchScore:    round3(identity.MatchScore),
			Minted:        minted,
		},
		TagsNote:          "tags are issuethemes.Theme's top-5 H-row loadings, not the full row (issuethemes exports no more; see internal/themes/identity.go)",
		CentroidTopIssues: centroidTopIssues(theme.Centroid, res.Means, storeIssues),
		TopIssuesByWeight: topIssuesByWeight(res.IssueThemes, index, storeIssues),
	}, nil
}

// DebugIssueThemesHandler serves GET /api/v1/debug/issues/{id}/themes.
type DebugIssueThemesHandler struct {
	Store issues.Store
	Cache *themes.Cache
	// RidgeDecomp classifies participation independent of whether the theme
	// cache has a usable factorization this revision — a degenerate theme
	// corpus can still have a perfectly usable decomposition to classify
	// against.
	RidgeDecomp *ridgedecomp.Cache
}

// Handle returns one issue's theme-membership row plus its participation
// status.
func (h DebugIssueThemesHandler) Handle(ctx context.Context, issueID string) (DebugIssueThemesResult, error) {
	if h.Store == nil {
		return DebugIssueThemesResult{}, errors.New("store is not available")
	}
	issue, err := h.Store.Get(ctx, issueID)
	if err != nil {
		return DebugIssueThemesResult{}, err
	}

	result := DebugIssueThemesResult{IssueID: issue.ID}

	if h.RidgeDecomp != nil {
		decomp, ok, err := h.RidgeDecomp.Current(ctx)
		if err != nil {
			return DebugIssueThemesResult{}, err
		}
		if ok {
			result.Participation = string(themes.ClassifyParticipation(decomp, issue))
		}
	}

	if h.Cache == nil {
		return result, nil
	}
	res, ok, err := h.Cache.Current(ctx)
	if err != nil {
		return DebugIssueThemesResult{}, err
	}
	if !ok {
		return result, nil
	}
	result.Computed = true

	for _, it := range res.IssueThemes {
		if it.IssueID != issue.ID {
			continue
		}
		weights := make([]DebugThemeWeight, 0, len(it.Weights))
		for i, w := range it.Weights {
			id := identityAt(res.Identities, i).StableID
			if id == "" {
				continue
			}
			weights = append(weights, DebugThemeWeight{StableID: id, Weight: round3(w)})
		}
		slices.SortStableFunc(weights, func(a, b DebugThemeWeight) int {
			if c := cmp.Compare(b.Weight, a.Weight); c != 0 {
				return c
			}
			return cmp.Compare(a.StableID, b.StableID)
		})
		result.Themes = weights
		break
	}
	return result, nil
}

func identityAt(identities []themes.ThemeIdentity, i int) themes.ThemeIdentity {
	if i < 0 || i >= len(identities) {
		return themes.ThemeIdentity{}
	}
	return identities[i]
}

func labelAt(labels []themes.ThemeLabel, i int) themes.ThemeLabel {
	if i < 0 || i >= len(labels) {
		return themes.ThemeLabel{}
	}
	return labels[i]
}

func totalThemeWeight(themeList []issuethemes.Theme) float64 {
	total := 0.0
	for _, t := range themeList {
		total += t.Weight
	}
	return total
}

func themeWeightShare(weight, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return weight / total
}

func toDebugThemeTags(tags []issuethemes.ThemeTag) []DebugThemeTag {
	out := make([]DebugThemeTag, len(tags))
	for i, t := range tags {
		out[i] = DebugThemeTag{Tag: t.Tag, Loading: round3(t.Loading)}
	}
	return out
}

func sortDebugThemeSummaries(summaries []DebugThemeSummary) {
	slices.SortStableFunc(summaries, func(a, b DebugThemeSummary) int {
		if c := cmp.Compare(b.Weight, a.Weight); c != 0 {
			return c
		}
		return cmp.Compare(a.StableID, b.StableID)
	})
}

// centroidTopIssues ranks every issue with a usable embedding by cosine
// similarity between the theme's centroid and the issue's own centered
// embedding — not the decomposition's tag-space reconstruction, matching the
// WP's "cosine(theme centroid, centered issue embedding)" spec. Deliberately
// not restricted to participating issues: the centroid describes a semantic
// direction, and an excluded issue's proximity to it is itself informative
// (e.g. "this no-anchor issue reads like the auth theme").
func centroidTopIssues(centroid []float64, means issuemath.CorpusMeans, storeIssues []issues.Issue) []DebugThemeIssueRef {
	if len(centroid) == 0 {
		return nil
	}
	type scored struct {
		id  string
		raw string
		sim float64
	}
	candidates := make([]scored, 0, len(storeIssues))
	for _, issue := range storeIssues {
		if len(issue.Embedding) == 0 {
			continue
		}
		centered := issuemath.CenterVector(issue.Embedding, means.Issue)
		if len(centered) != len(centroid) {
			continue
		}
		candidates = append(candidates, scored{
			id:  issue.ID,
			raw: truncateRaw(issue.Raw),
			sim: vectors.CosineSimilarity(centroid, centered),
		})
	}
	slices.SortStableFunc(candidates, func(a, b scored) int {
		if c := cmp.Compare(b.sim, a.sim); c != 0 {
			return c
		}
		return cmp.Compare(a.id, b.id)
	})
	if len(candidates) > debugThemeTopIssues {
		candidates = candidates[:debugThemeTopIssues]
	}
	out := make([]DebugThemeIssueRef, len(candidates))
	for i, c := range candidates {
		out[i] = DebugThemeIssueRef{ID: c.id, Raw: c.raw, Score: round3(c.sim)}
	}
	return out
}

// topIssuesByWeight ranks the theme's participating issues by their raw W
// entry at the theme's index — the factorization's own membership weight,
// not a derived similarity.
func topIssuesByWeight(issueThemes []issuethemes.IssueThemes, index int, storeIssues []issues.Issue) []DebugThemeIssueRef {
	rawByID := make(map[string]string, len(storeIssues))
	for _, issue := range storeIssues {
		rawByID[issue.ID] = truncateRaw(issue.Raw)
	}
	type scored struct {
		id     string
		raw    string
		weight float64
	}
	candidates := make([]scored, 0, len(issueThemes))
	for _, it := range issueThemes {
		if index < 0 || index >= len(it.Weights) {
			continue
		}
		candidates = append(candidates, scored{id: it.IssueID, raw: rawByID[it.IssueID], weight: it.Weights[index]})
	}
	slices.SortStableFunc(candidates, func(a, b scored) int {
		if c := cmp.Compare(b.weight, a.weight); c != 0 {
			return c
		}
		return cmp.Compare(a.id, b.id)
	})
	if len(candidates) > debugThemeTopIssues {
		candidates = candidates[:debugThemeTopIssues]
	}
	out := make([]DebugThemeIssueRef, len(candidates))
	for i, c := range candidates {
		out[i] = DebugThemeIssueRef{ID: c.id, Raw: c.raw, Score: round3(c.weight)}
	}
	return out
}
