# Content Confidence Score - V1 Spec

## Overview

Add a bounded `0..1` content-confidence score for issues. This score measures how trustworthy the canonical issue text is as input to downstream tagging, embedding, and retrieval.

This is a narrower sub-spec under [scoring-spec.md](./scoring-spec.md). If the two documents disagree, the broader scoring spec wins.

## Current Problems

The current system treats all issue text as equally trustworthy once an issue exists.

That causes several bad outcomes:

- a one-line vague issue can influence similarity and ranking as much as a detailed report
- a long but repetitive or boilerplate-heavy issue can look richer than it really is
- downstream consumers have no clean way to distinguish weak text from strong text

## Decision Summary

The v1 score should be:

- deterministic
- computed from canonical `issue.raw` text only
- bounded and saturating
- cheap to compute
- interpreted as confidence, not importance

The first consumer should be search. Weighted PCA, map layout, and broader embedding weighting should wait until the primitive is validated.

## Canonical Interpretation

`content_confidence(issue)` answers:

> How much trustworthy signal exists in this issue's canonical text for tagging, embeddings, and retrieval?

Higher confidence means:

- there is enough text to anchor meaning
- the text contains varied, content-bearing terms
- the text has useful structure or technical detail
- the text is not mostly repetition

Lower confidence means:

- the text is very short
- the text is generic or underspecified
- the text is long but repetitive

## Non-Goals

This score is not:

- issue importance
- issue priority
- issue urgency
- maturity
- stability

Longer text should increase confidence only up to a cap.

## V1 Inputs

V1 should use only cheap features from canonical issue text:

- token count
- content-token diversity
- structural richness
- repetition / boilerplate penalty

No heavy NLP, no model calls, and no issue-history inputs in v1.

## Text Normalization

Before feature extraction:

1. Trim surrounding whitespace.
2. Normalize internal line endings to `\n`.
3. Collapse repeated blank lines to a single blank line.
4. Extract tokens with a deterministic regex such as `[A-Za-z0-9_./:-]+`.
5. Lowercase tokens for counting and diversity features.

The score should be computed from canonical `issue.raw`, not from the full discussion history.

## V1 Formula

The v1 score should be:

```text
confidence =
  clamp(
    0.25 +
    0.45 * length_signal +
    0.15 * diversity_signal +
    0.20 * structure_signal -
    0.25 * repetition_penalty,
    0,
    1,
  )
```

### 1. Length Signal

Use a saturating transform:

```text
length_signal = 1 - exp(-token_count / 80)
```

Interpretation:

- very short text gets little benefit
- medium-length text gains quickly
- long text approaches a cap instead of increasing without bound

### 2. Diversity Signal

Measure how varied the content-bearing vocabulary is.

Steps:

1. Keep tokens with length `>= 3`.
2. Compute:

```text
unique_ratio = unique_content_tokens / max(content_token_count, 1)
```

3. Convert to a bounded signal:

```text
diversity_signal = clamp((unique_ratio - 0.25) / 0.45, 0, 1)
```

Interpretation:

- repeated generic phrasing stays low
- varied content-bearing text rises

### 3. Structure Signal

Count up to four cheap structure/detail features:

- `multi_sentence`: at least two sentence boundaries
- `multi_line`: at least two non-empty lines
- `list_like`: a line starts with `-`, `*`, or `1.` / `1)`
- `technical_detail`: contains any of:
  - backticks
  - a slash or path-like token
  - a version-like or numeric token
  - an error-like keyword such as `error`, `exception`, `trace`, `stack`

Then compute:

```text
structure_signal = matched_features / 4
```

Interpretation:

- structure is evidence that the issue contains actionable detail
- this is a bonus, not the primary driver

### 4. Repetition Penalty

Apply a penalty when text is long but low-signal because it repeats itself.

Use two bounded sub-signals:

- `duplicate_line_signal`
  - fraction of non-empty lines that are exact duplicates after trim/lowercase
- `low_diversity_signal`
  - `1` when `token_count >= 12` and `unique_ratio < 0.35`, else `0`

Then compute:

```text
repetition_penalty = clamp(max(duplicate_line_signal, low_diversity_signal), 0, 1)
```

This keeps the penalty simple and deterministic.

## Expected Behavior

The score should order examples roughly like this:

- `"fix login bug"` -> low confidence
- `"Safari export fails after tapping Share twice"` -> low-to-medium confidence
- short paragraph with repro steps and observed/expected behavior -> medium confidence
- detailed issue with structured repro, environment details, and technical markers -> high confidence
- long repeated boilerplate -> below a genuinely informative issue of similar length

Exact numeric thresholds may shift during implementation, but these relative orderings should not.

## Consumer Plan

### First Consumer: Search

Use content confidence first in search because the semantics are cleanest there.

V1 search application:

- keep the current relevance blend as the primary score
- use `content_confidence` as a tie-breaker when candidate scores are close
- suggested tie window:
  `abs(score_a - score_b) <= 0.05`
- within that window, prefer higher-confidence issue text

This avoids over-baking a new primitive into ranking before it has been validated.

### Deferred Consumers

Do not use v1 content confidence yet for:

- weighted PCA covariance
- map position computation
- map edge weighting
- tag-score rescaling

Those are valid later consumers, but they should wait until the primitive is trusted.

## Implementation Plan

### Step 1. Pure Signal Implementation

Add a pure deterministic scorer in `internal/issues`, for example:

- `internal/issues/content_confidence.go`
- `internal/issues/content_confidence_test.go`

Suggested API:

```go
func ComputeContentConfidence(raw string) float64
```

If feature inspection is useful for tests and debug UIs, also expose:

```go
type ContentConfidenceFeatures struct {
    TokenCount         int
    UniqueRatio        float64
    StructureSignal    float64
    RepetitionPenalty  float64
}
```

### Step 2. Search Integration

Integrate the signal into search ordering before any broader rollout.

Recommended starting point:

- compute confidence per candidate issue from canonical `raw`
- keep existing blended similarity score unchanged as the primary rank term
- use confidence only as a tie-breaker for near-equal results

Likely touch points:

- `internal/map/search.go`
- `internal/queries/search_issues.go`
- search-related API tests

### Step 3. Optional Exposure

Do not add a database column in v1.

If the signal proves useful after search validation, expose it on issue responses as a derived field and then consider whether persistence is worth the complexity.

## Required Tests

### Unit Tests

Add fixtures covering:

- empty string -> very low confidence
- one-line vague issue -> low confidence
- one-line specific but short issue -> above vague issue
- medium structured issue -> above short issues
- long detailed issue -> high confidence
- long repetitive issue -> below equally long informative issue
- list-structured issue -> above same-token unstructured issue when other inputs are similar

### Ordering Tests

Assert relative ordering rather than brittle exact values:

- detailed > medium > short specific > short vague
- detailed > long repetitive
- structured > unstructured when token count is similar

### Search Integration Tests

Add at least one search test showing:

- two near-equal search candidates
- the richer issue outranks the vaguer issue because of content confidence

The test should avoid proving that confidence overrides obviously better relevance. It should only break near ties.

## Open Questions For Later

- whether confidence should eventually be exposed in issue detail APIs
- whether confidence should be computed for query text separately from issue text
- whether later versions should detect richer repro semantics more explicitly
- whether search should eventually use confidence as a mild multiplier instead of only a tie-breaker
