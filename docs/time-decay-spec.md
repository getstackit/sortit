# Time Decay Spec

## Goal

Define a contextual freshness weight for surfaces where recency matters.

Time decay is:

- a contextual modifier
- bounded
- deterministic

Time decay is not:

- quality
- maturity
- authority
- velocity

## Canonical v1 definition

Use a bounded exponential decay with a nonzero floor:

- floor: `0.3`
- half-life: `90 days`

Formula:

- `timeWeight = floor + (1 - floor) * exp(-ln(2) * age_days / half_life_days)`

This uses a true half-life. After `90` days, the decaying portion contributes half as much as it did at day `0`.

## Timestamp choice

For issues, `age` is measured from the latest observed issue activity timestamp:

- issue `createdAt`
- newest discussion post
- newest link event
- `closedAt` when present

This makes time decay represent recency of the issue state rather than raw creation date alone.

## Surface-specific use

- Search:
  multiply combined relevance by `timeWeight`
- Explore / related:
  multiply by `sqrt(timeWeight)` for gentler decay
- Person recommendations:
  multiply candidate issue score by `timeWeight`
- Map layout:
  do not apply time decay

## Non-goals

Do not use v1 time decay for:

- canonical quality scores
- tag specificity
- map positioning

## Test fixtures

Cover at least:

- recent issue outranks stale issue when base similarity is equal
- recent activity revives an old issue
- zero timestamp falls back to neutral weight
- the 90-day point produces the expected half-life value
