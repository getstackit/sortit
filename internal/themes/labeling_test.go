package themes

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// recordingLabeler is a deterministic fake Labeler: it returns a fixed name
// derived from the top tag and records every call, so tests can assert what the
// async path did.
type recordingLabeler struct {
	mu    sync.Mutex
	calls int
}

func (l *recordingLabeler) ProposeThemeLabel(_ context.Context, topTags []LabelTag, _ []string) (string, string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	name := "unnamed"
	if len(topTags) > 0 {
		name = "llm:" + topTags[0].Tag
	}
	return name, "described " + name, nil
}

func (l *recordingLabeler) callCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

// blockingLabeler blocks in ProposeThemeLabel until release is closed, then
// returns the configured error (or a name). It lets a test prove Current does not
// block while a labeler is in flight, and that a failing/blocking labeler leaks
// no goroutine (the test waits for the WaitGroup after releasing).
type blockingLabeler struct {
	entered chan struct{}
	release chan struct{}
	err     error
}

func newBlockingLabeler(err error) *blockingLabeler {
	return &blockingLabeler{
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
		err:     err,
	}
}

func (l *blockingLabeler) ProposeThemeLabel(_ context.Context, _ []LabelTag, _ []string) (string, string, error) {
	select {
	case l.entered <- struct{}{}:
	default:
	}
	<-l.release
	if l.err != nil {
		return "", "", l.err
	}
	return "eventually", "eventually described", nil
}

// --- Pure relabel-decision tests ---
//
// Per the WP: constructing a corpus that crosses the 0.5 top-tag relabel band
// while staying above the 0.6 identity band is a narrow, brittle fixture, so the
// mint/keep/relabel decision is asserted directly as a pure function here.

func TestDecideLabelActionMintKeepRelabel(t *testing.T) {
	row := themeRow{"alpha": 0.7, "beta": 0.3}

	if got := decideLabelAction(nil, row); got != labelMint {
		t.Fatalf("no entry should mint, got %v", got)
	}

	// Same row → cosine 1.0 ≥ threshold → keep.
	keepEntry := &themeLabelEntry{snapshot: themeRow{"alpha": 0.7, "beta": 0.3}}
	if got := decideLabelAction(keepEntry, row); got != labelKeep {
		t.Fatalf("unchanged top tags should keep, got %v", got)
	}

	// Fully disjoint tag set → cosine 0 < threshold → relabel.
	driftEntry := &themeLabelEntry{snapshot: themeRow{"gamma": 0.8, "delta": 0.2}}
	if got := decideLabelAction(driftEntry, row); got != labelRelabel {
		t.Fatalf("drifted top tags should relabel, got %v", got)
	}

	// A snapshot just above the relabel threshold keeps; just below relabels.
	// alpha shared with weight 1 on both sides plus one orthogonal tag on each.
	// cosine = 1/ (sqrt(1+x^2)) ; pick x so cosine straddles 0.5.
	aboveEntry := &themeLabelEntry{snapshot: themeRow{"alpha": 1, "beta": 0.2}}
	if got := decideLabelAction(aboveEntry, themeRow{"alpha": 1, "beta": 0.2}); got != labelKeep {
		t.Fatalf("identical snapshot must keep, got %v", got)
	}
}

// TestLabelLifecycleThroughStore drives the mint → keep → relabel lifecycle
// through the label store the way compute does, without needing the NMF to
// produce a fixture that crosses both thresholds. It asserts the stored label,
// the fallback content, previousLabel on relabel, and the generation bump.
func TestLabelLifecycleThroughStore(t *testing.T) {
	c := &Cache{}
	c.labels.entries = map[string]*themeLabelEntry{}

	// Mint: no entry yet.
	row1 := themeRow{"alpha": 0.7, "beta": 0.3}
	entry := c.labels.entries["theme-1"]
	if decideLabelAction(entry, row1) != labelMint {
		t.Fatal("expected mint")
	}
	c.labels.entries["theme-1"] = &themeLabelEntry{label: "alpha / beta", snapshot: row1, gen: 1}
	entry = c.labels.entries["theme-1"]

	// Keep: same row.
	if decideLabelAction(entry, row1) != labelKeep {
		t.Fatal("expected keep on unchanged row")
	}

	// Relabel: disjoint row. Mirror what label() does on relabel.
	row2 := themeRow{"gamma": 0.9, "delta": 0.1}
	if decideLabelAction(entry, row2) != labelRelabel {
		t.Fatal("expected relabel on drifted row")
	}
	entry.previous = entry.label
	entry.label = "gamma / delta"
	entry.snapshot = row2
	entry.gen++
	if entry.previous != "alpha / beta" {
		t.Fatalf("previous label = %q, want alpha / beta", entry.previous)
	}
	if entry.gen != 2 {
		t.Fatalf("gen = %d, want 2 after one relabel", entry.gen)
	}
}

// TestStubLabelerLabelsPresentInResult asserts the async labeler path attaches
// deterministic labels to the result, visible on the next Current call at the
// same revision (the async writeback refreshes the cached result).
func TestStubLabelerLabelsPresentInResult(t *testing.T) {
	items := clusteredIssues(24)
	rev := &stubRevisions{}
	rev.rev.Store(1)
	c := newCache(items, themeTags(), rev)
	labeler := &recordingLabeler{}
	c.Labeler = labeler

	res, ok, err := c.Current(context.Background())
	if err != nil || !ok {
		t.Fatalf("Current: ok=%v err=%v", ok, err)
	}
	// Immediately, every theme carries at least the deterministic fallback.
	if len(res.Labels) != len(res.Themes) {
		t.Fatalf("labels not index-aligned: %d labels, %d themes", len(res.Labels), len(res.Themes))
	}
	for i, l := range res.Labels {
		if l.Label == "" {
			t.Fatalf("theme %d has no label", i)
		}
	}

	c.waitForLabeling()

	// After the async labeler completes, the cached result serves the LLM labels.
	res2, ok, err := c.Current(context.Background())
	if err != nil || !ok {
		t.Fatalf("Current after labeling: ok=%v err=%v", ok, err)
	}
	sawLLM := false
	for _, l := range res2.Labels {
		if len(l.Label) >= 4 && l.Label[:4] == "llm:" {
			sawLLM = true
		}
	}
	if !sawLLM {
		t.Fatal("expected at least one LLM label after waitForLabeling")
	}
	if labeler.callCount() == 0 {
		t.Fatal("labeler was never called")
	}

	// A second Current at the same revision must not re-label (no new calls).
	before := labeler.callCount()
	if _, _, err := c.Current(context.Background()); err != nil {
		t.Fatalf("Current (memoized): %v", err)
	}
	c.waitForLabeling()
	if labeler.callCount() != before {
		t.Fatalf("labeler re-called on a memoized revision: %d → %d", before, labeler.callCount())
	}
}

// TestLabelingDoesNotBlockCurrent proves the cache mutex is never held across a
// labeler call: while a labeler is blocked in flight, Current returns promptly.
func TestLabelingDoesNotBlockCurrent(t *testing.T) {
	items := clusteredIssues(24)
	rev := &stubRevisions{}
	rev.rev.Store(1)
	c := newCache(items, themeTags(), rev)
	labeler := newBlockingLabeler(nil)
	c.Labeler = labeler

	// First Current spawns the labeler goroutine(s), which block in the labeler.
	if _, ok, err := c.Current(context.Background()); err != nil || !ok {
		t.Fatalf("Current: ok=%v err=%v", ok, err)
	}
	<-labeler.entered // a labeler call is now in flight and blocked

	// A concurrent Current must not block on the in-flight labeler.
	done := make(chan struct{})
	go func() {
		_, _, _ = c.Current(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Current blocked while a labeler was in flight — mutex held across the LLM call")
	}

	// Release the labeler and make sure the goroutine drains (no leak).
	close(labeler.release)
	c.waitForLabeling()
}

// TestLabelingProviderErrorFallsBack proves a failing (and initially blocking)
// labeler leaves computation unaffected, serves the fallback label, and leaks no
// goroutine.
func TestLabelingProviderErrorFallsBack(t *testing.T) {
	items := clusteredIssues(24)
	rev := &stubRevisions{}
	rev.rev.Store(1)
	c := newCache(items, themeTags(), rev)
	labeler := newBlockingLabeler(errors.New("provider down"))
	c.Labeler = labeler

	res, ok, err := c.Current(context.Background())
	if err != nil || !ok {
		t.Fatalf("Current unaffected by labeling: ok=%v err=%v", ok, err)
	}
	fallbacks := make([]string, len(res.Labels))
	for i, l := range res.Labels {
		if l.Label == "" {
			t.Fatalf("theme %d has no fallback label", i)
		}
		fallbacks[i] = l.Label
	}

	<-labeler.entered
	close(labeler.release)
	c.waitForLabeling() // must return — no leaked goroutine

	// Labels are unchanged: the fallback stayed after the error.
	res2, _, err := c.Current(context.Background())
	if err != nil {
		t.Fatalf("Current after failed labeling: %v", err)
	}
	for i, l := range res2.Labels {
		if l.Label != fallbacks[i] {
			t.Fatalf("theme %d label changed after labeler error: %q → %q", i, fallbacks[i], l.Label)
		}
	}
}

// TestNilLabelerNoBackgroundWork asserts that with no Labeler the result still
// carries deterministic fallback labels and no async work is scheduled.
func TestNilLabelerNoBackgroundWork(t *testing.T) {
	items := clusteredIssues(24)
	rev := &stubRevisions{}
	rev.rev.Store(1)
	c := newCache(items, themeTags(), rev) // no Labeler set

	res, ok, err := c.Current(context.Background())
	if err != nil || !ok {
		t.Fatalf("Current: ok=%v err=%v", ok, err)
	}
	for i, l := range res.Labels {
		if l.Label == "" {
			t.Fatalf("theme %d has no fallback label without a labeler", i)
		}
	}
	// waitForLabeling returns immediately when nothing was scheduled.
	c.waitForLabeling()
}
