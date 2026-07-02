package themes

import (
	"context"
	"sort"
	"strings"
	"time"

	"sortit/internal/issuemath"
	"sortit/internal/issues"
	"sortit/internal/issuethemes"
	"sortit/internal/vectors"
)

// themeRelabelThreshold is the minimum cosine (over the theme's top-tag loading
// row) between a theme's current top tags and the top tags it carried when it
// was last labeled. Above it the theme keeps its label; below it the tags have
// drifted far enough that the old name no longer describes the theme, so it is
// relabeled (keeping the old name as previousLabel for UI continuity).
//
// It is deliberately LOWER than the 0.6 themeMatchThreshold that governs stable
// identity: a theme keeps its ID through more drift than it keeps its label. The
// ordering matters — a theme should re-match (stay the same theme) more readily
// than it re-labels (get a new name), so an ID rarely churns while its label can
// refresh when the underlying tags have genuinely moved. 0.5 is a starting point;
// revisit with WP-206 telemetry alongside themeMatchThreshold.
const themeRelabelThreshold = 0.5

// labelFallbackTags caps how many top tags the deterministic fallback label
// joins (e.g. "billing / payments / export"), keeping it to the "≤ ~4 words" the
// labeler contract targets. It is the immediate label a theme carries until an
// async LLM label (if any) replaces it.
const labelFallbackTags = 3

// labelTitleCount caps the centroid-nearest issue titles handed to the labeler
// as grounding, matching the "handful" the design asks for.
const labelTitleCount = 5

// labelTitleMaxChars truncates each grounding title so a long issue body doesn't
// bloat the label prompt.
const labelTitleMaxChars = 120

// labelTimeout backstops a hung labeler so a stuck provider can never leak a
// goroutine forever; the async job errors out and the theme keeps its fallback.
const labelTimeout = 60 * time.Second

// LabelTag is one tag loading passed to the Labeler — a themes-local mirror of
// ai.ThemeLabelTag, so this package stays decoupled from internal/ai (the
// production Labeler is an adapter that assembles the concept frame and delegates
// to the analyzer, exactly as IssueLister decouples the cache from the store).
type LabelTag struct {
	Tag     string
	Loading float64
}

// Labeler names and describes a theme from its top tags (with loadings) and a
// few centroid-nearest issue titles. Implementations own concept-frame grounding.
// It is optional on the Cache; when absent every theme keeps its deterministic
// top-tag fallback label. Labeling runs asynchronously and must never be called
// while the cache mutex is held.
type Labeler interface {
	ProposeThemeLabel(ctx context.Context, topTags []LabelTag, issueTitles []string) (name string, description string, err error)
}

// ThemeLabel is a theme's human-facing name and description, plus the previous
// name retained across a relabel so a consumer can show continuity.
type ThemeLabel struct {
	// Label is the theme's short display name — an LLM name when the Labeler has
	// produced one, otherwise the deterministic top-tag fallback.
	Label string
	// Description is a one-sentence summary; empty until the LLM label lands (the
	// fallback carries no description).
	Description string
	// PreviousLabel is the label the theme carried before its most recent
	// relabel, kept for UI continuity. Empty for a theme that has never relabeled.
	PreviousLabel string
}

// themeLabelStore is the in-memory theme-label memory keyed by stable theme ID.
// It holds, per theme, the current label/description, the previous label, the
// top-tags snapshot the label was assigned against (the relabel baseline), and a
// generation counter bumped on every mint/relabel so a stale async writeback can
// detect it has been superseded.
type themeLabelStore struct {
	entries map[string]*themeLabelEntry
}

type themeLabelEntry struct {
	label       string
	description string
	previous    string
	// snapshot is the theme's top-tag loading row at the time this label was
	// assigned — the baseline the relabel-on-drift check compares the current row
	// against.
	snapshot themeRow
	// gen increments on each mint/relabel. An async label job captures the gen it
	// was launched for and applies its result only if the entry's gen still
	// matches, so a job racing a newer relabel is discarded instead of clobbering
	// the fresher label.
	gen int
}

// labelJob is one unit of async labeling work: the stable ID and generation it
// targets plus the immutable inputs (top tags + grounding titles) captured under
// the mutex so the LLM call itself touches no shared state.
type labelJob struct {
	id     string
	gen    int
	tags   []LabelTag
	titles []string
}

// labelAction is the lifecycle decision for one theme this revision.
type labelAction int

const (
	// labelKeep: the theme's top tags have not drifted past the relabel
	// threshold, so it keeps its existing label untouched.
	labelKeep labelAction = iota
	// labelMint: the theme has no label yet (a freshly minted stable ID) and
	// needs its first one.
	labelMint
	// labelRelabel: the theme kept its stable ID but its top tags drifted below
	// the relabel threshold, so it earns a new label (old one kept as previous).
	labelRelabel
)

// decideLabelAction is the pure relabel decision: no entry means mint; an entry
// whose snapshot still matches the current row within threshold means keep;
// otherwise relabel. Kept a free function so the mint/keep/relabel band can be
// tested directly without constructing a corpus that crosses both the identity
// and relabel thresholds at once (a narrow, hard-to-build band — see the WP).
func decideLabelAction(existing *themeLabelEntry, row themeRow) labelAction {
	if existing == nil {
		return labelMint
	}
	if cosine(existing.snapshot, row) >= themeRelabelThreshold {
		return labelKeep
	}
	return labelRelabel
}

// label attaches per-theme labels to a freshly identified result and, when a
// Labeler is configured, spawns background jobs to name minted/relabeled themes.
// It runs under c.mu (called from compute): it mutates the label store and reads
// the store to fill result.Labels, but it never calls the Labeler here — the
// goroutines it spawns do that, acquiring c.mu only after the LLM call returns,
// so the mutex is never held across a labeler call. Called only from compute.
func (c *Cache) label(result *Result, items []issues.Issue) {
	if c.labels.entries == nil {
		c.labels.entries = make(map[string]*themeLabelEntry)
	}

	labels := make([]ThemeLabel, len(result.Themes))
	var jobs []labelJob
	for i := range result.Themes {
		theme := result.Themes[i]
		id := identityStableID(result.Identities, i)
		if id == "" {
			continue
		}
		row := themeRowFrom(theme)
		entry := c.labels.entries[id]

		switch decideLabelAction(entry, row) {
		case labelMint:
			entry = &themeLabelEntry{label: fallbackLabel(theme.Tags), snapshot: row, gen: 1}
			c.labels.entries[id] = entry
			if job, ok := c.enqueueLabelJobLocked(id, entry.gen, theme, result.Means, items); ok {
				jobs = append(jobs, job)
			}
		case labelRelabel:
			entry.previous = entry.label
			entry.label = fallbackLabel(theme.Tags)
			entry.description = ""
			entry.snapshot = row
			entry.gen++
			if job, ok := c.enqueueLabelJobLocked(id, entry.gen, theme, result.Means, items); ok {
				jobs = append(jobs, job)
			}
		case labelKeep:
			// Keeps its existing label; nothing to do.
		}

		labels[i] = ThemeLabel{Label: entry.label, Description: entry.description, PreviousLabel: entry.previous}
	}
	result.Labels = labels

	// Spawn after the store is fully updated. Launching goroutines while holding
	// c.mu is fine — they block on c.mu only after the LLM call returns.
	for _, job := range jobs {
		c.labelWG.Add(1)
		go c.runLabelJob(job)
	}
}

// enqueueLabelJobLocked builds a label job for a theme and marks it in flight,
// unless there is no Labeler or a job for this ID+generation is already running.
// Called under c.mu.
func (c *Cache) enqueueLabelJobLocked(id string, gen int, theme issuethemes.Theme, means issuemath.CorpusMeans, items []issues.Issue) (labelJob, bool) {
	if c.Labeler == nil {
		return labelJob{}, false
	}
	if c.labelInFlight == nil {
		c.labelInFlight = make(map[string]int)
	}
	if c.labelInFlight[id] == gen {
		return labelJob{}, false
	}
	c.labelInFlight[id] = gen
	return labelJob{
		id:     id,
		gen:    gen,
		tags:   labelTags(theme.Tags),
		titles: centroidNearestTitles(theme.Centroid, means, items, labelTitleCount),
	}, true
}

// runLabelJob calls the Labeler off the mutex, then writes the result back under
// the mutex — the only place the LLM name/description reaches the store. A stale
// result (the entry was relabeled while this job ran, so its generation moved on)
// or an error/empty name is discarded, leaving the fallback in place. On success
// it refreshes the cached result's labels so the next Current call (even at the
// same revision) serves the LLM name.
func (c *Cache) runLabelJob(job labelJob) {
	defer c.labelWG.Done()

	ctx, cancel := context.WithTimeout(context.Background(), labelTimeout)
	defer cancel()
	name, description, err := c.Labeler.ProposeThemeLabel(ctx, job.tags, job.titles)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.labelInFlight[job.id] == job.gen {
		delete(c.labelInFlight, job.id)
	}
	if err != nil {
		return // labeling failed — the deterministic fallback stays
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	entry, ok := c.labels.entries[job.id]
	if !ok || entry.gen != job.gen {
		return // superseded by a newer mint/relabel
	}
	entry.label = name
	entry.description = strings.TrimSpace(description)
	c.reattachLabelsLocked()
}

// reattachLabelsLocked rebuilds the cached result's Labels slice from the current
// label store, so an async label update is visible to subsequent Current calls at
// the same revision. It always assigns a fresh slice (never mutates the existing
// backing array), so a Result already returned to a caller keeps its own stable
// snapshot. Called under c.mu.
func (c *Cache) reattachLabelsLocked() {
	if !c.ok {
		return
	}
	labels := make([]ThemeLabel, len(c.result.Themes))
	for i := range c.result.Themes {
		id := identityStableID(c.result.Identities, i)
		if id == "" {
			continue
		}
		if entry, ok := c.labels.entries[id]; ok {
			labels[i] = ThemeLabel{Label: entry.label, Description: entry.description, PreviousLabel: entry.previous}
		}
	}
	c.result.Labels = labels
}

// fallbackLabel is the deterministic label a theme carries before (or without) an
// LLM name: its heaviest tag names joined with " / " (e.g. "billing / payments").
func fallbackLabel(tags []issuethemes.ThemeTag) string {
	names := make([]string, 0, labelFallbackTags)
	for _, tag := range tags {
		name := strings.TrimSpace(tag.Tag)
		if name == "" {
			continue
		}
		names = append(names, name)
		if len(names) >= labelFallbackTags {
			break
		}
	}
	return strings.Join(names, " / ")
}

// labelTags projects the factorizer's top-tag slice onto the Labeler input type.
func labelTags(tags []issuethemes.ThemeTag) []LabelTag {
	out := make([]LabelTag, 0, len(tags))
	for _, tag := range tags {
		name := strings.TrimSpace(tag.Tag)
		if name == "" {
			continue
		}
		out = append(out, LabelTag{Tag: name, Loading: tag.Loading})
	}
	return out
}

// centroidNearestTitles returns up to n issue titles whose centered embedding is
// most cosine-similar to the theme's centroid — the same centroid-nearest lookup
// the debug detail endpoint uses, reused here to ground the label prompt. Not
// restricted to participating issues: proximity to the semantic direction is what
// matters. Deterministic (sim desc, ID tiebreak).
func centroidNearestTitles(centroid []float64, means issuemath.CorpusMeans, items []issues.Issue, n int) []string {
	if len(centroid) == 0 || n <= 0 {
		return nil
	}
	type scored struct {
		id    string
		title string
		sim   float64
	}
	candidates := make([]scored, 0, len(items))
	for _, issue := range items {
		if len(issue.Embedding) == 0 {
			continue
		}
		title := strings.TrimSpace(issue.Raw)
		if title == "" {
			continue
		}
		centered := issuemath.CenterVector(issue.Embedding, means.Issue)
		if len(centered) != len(centroid) {
			continue
		}
		candidates = append(candidates, scored{
			id:    issue.ID,
			title: truncateTitle(title),
			sim:   vectors.CosineSimilarity(centroid, centered),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].sim != candidates[j].sim {
			return candidates[i].sim > candidates[j].sim
		}
		return candidates[i].id < candidates[j].id
	})
	if len(candidates) > n {
		candidates = candidates[:n]
	}
	titles := make([]string, len(candidates))
	for i, c := range candidates {
		titles[i] = c.title
	}
	return titles
}

func truncateTitle(text string) string {
	runes := []rune(text)
	if len(runes) <= labelTitleMaxChars {
		return text
	}
	return strings.TrimSpace(string(runes[:labelTitleMaxChars])) + "…"
}

// waitForLabeling blocks until all outstanding async label jobs have finished.
// It exists for tests (deterministic assertions and goroutine-leak checks);
// production never waits — labeling is fire-and-forget.
func (c *Cache) waitForLabeling() {
	c.labelWG.Wait()
}

// identityStableID returns the stable ID for the theme at index i, or "" when the
// index is out of the identities range.
func identityStableID(identities []ThemeIdentity, i int) string {
	if i < 0 || i >= len(identities) {
		return ""
	}
	return identities[i].StableID
}
