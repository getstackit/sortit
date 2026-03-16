# Velocity Spec

## Goal

Define a bounded, deterministic `0..1` signal for how actively an issue is being worked right now.

Velocity is:

- recent activity rate
- contextual
- surface-specific

Velocity is not:

- quality
- maturity
- freshness
- importance

## Canonical v1 definition

`velocity` is derived at read time from recent meaningful events on the issue:

- refinement posts
- progress posts
- issue link creation events

Excluded event types:

- initial report
- assignment events
- close / reopen events

## Formula

Use a rolling 30-day window with exponential recency decay.

For each event in the last 30 days:

- refinement weight: `1.0`
- progress weight: `0.85`
- link weight: `0.65`
- decay: `exp(-ln(2) * age_days / 14)`

Then:

- `weighted_recent_activity = sum(weight * decay)`
- `velocity = 1 - exp(-weighted_recent_activity / 2.5)`

Clamp the final value to `0..1`.

Also expose:

- `recentActivityCount`

This is the raw count of included events in the 30-day window.

## Interpretation

- high velocity: the issue is receiving multiple recent meaningful updates
- low velocity: the issue is quiet or dormant
- zero velocity: no recent meaningful activity in the 30-day window

## Initial consumers

- Issue detail timeline
- Search ranking: mild active-discussion boost
- Person recommendations: mild penalty for already-busy issues
- Explore opportunities: mild boost for emergent active clusters

## Non-goals

Do not use v1 velocity for:

- global issue quality
- PCA weighting
- canonical authority
- maturity directly

## Test fixtures

Cover at least:

- no recent activity
- only old activity
- recent refinement beats old refinement
- mixed recent posts and links beat a single recent event
- initial report alone does not create velocity
