# Resolution-Aware Weighting Spec

## Goal

Define a small, deterministic contextual modifier from `closed_reason`.

Resolution-aware weighting is:

- contextual
- bounded
- deterministic

Resolution-aware weighting is not:

- issue quality
- authority
- a replacement for duplicate / canonical link semantics

## Canonical v1 definition

Use `closed_reason` only as a per-surface modifier.

Do not let `closed_reason` define canonical hiding by itself. Link semantics remain the source of truth for:

- `duplicate_of`
- `merged_into`

## Close reasons

The current allowed values are:

- `fixed`
- `stale`
- `duplicate`
- `wont_fix`
- `by_design`

## v1 weights

For search and related surfaces:

- open issue: `1.0`
- closed `fixed`: `0.55`
- closed `stale`: `0.2`
- closed `duplicate`: `0.15`
- closed `wont_fix`: `0.35`
- closed `by_design`: `0.35`
- unknown closed reason: `0.4`

These are not global truth values. They are search-context defaults.

## Semantics

- `fixed`:
  useful reference material, but weaker than active open work
- `stale`:
  usually noisy or underdeveloped, strong penalty
- `duplicate`:
  weak standalone value; canonical link target should do the real work
- `wont_fix` and `by_design`:
  useful explanatory context, but not primary candidates

## Surface-specific use

- Search:
  multiply blended relevance by resolution weight
- Explore / related:
  optionally apply a gentler form after search validation
- Person recommendations:
  no v1 use; closed issues are already excluded
- Map layout / PCA:
  do not apply in this issue; downstream consumers can read this later

## Non-goals

Do not use v1 resolution-aware weighting for:

- tag specificity
- content confidence
- maturity
- velocity
- map positioning
- replacing link-based canonical resolution

## Implementation notes

- `closed_reason == duplicate` should not try to reconstruct canonical targets from reason text alone
- if an issue is closed as duplicate but lacks canonical links, treat it as heavily downweighted, not hidden
- search and unified search should be the first consumers

## Test fixtures

Cover at least:

- open issue outranks fixed issue when base match is equal
- fixed issue outranks stale issue when base match is equal
- stale issue outranks duplicate issue when base match is equal
- duplicate / merged link semantics still hide non-canonical issues independently of `closed_reason`
