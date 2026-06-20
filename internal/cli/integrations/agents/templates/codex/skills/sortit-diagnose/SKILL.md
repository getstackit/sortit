---
name: sortit-diagnose
description: Diagnose the factor model quality and identify issues where the tag taxonomy is failing. Trigger phrases include "improve the tags", "what's our R²", "why are these issues badly tagged", "find issues that need re-enrichment", "diagnose the factor model", "which tags are misclassified", and "find clusters of poorly explained issues".
---

# Sortit Diagnose

Inspect factor model quality and identify actionable improvements to the tag taxonomy.

## Commands

### Corpus-wide diagnostics

```bash
command sortit debug factor-weights
```

Shows:
- **aggregateR2**: how much variance the tag structure explains overall (higher = better taxonomy)
- **factorWeight / residualWeight**: data-driven blend weights
- **lowR2Issues**: issues poorly explained by tags, sorted worst-first

### Per-issue deep dive

```bash
command sortit debug issue-r2 <issue-id>
```

Shows:
- **r2**: what fraction of this issue's embedding the tags explain
- **tags**: each assigned tag with relevance and alignment (cosine similarity between the tag embedding and the issue embedding)
- **nearestResidualTags**: catalog tags closest to the unexplained residual — candidates for assignment
- **residualNeighbors**: other issues whose residuals point the same direction — they share a missing concept
- **diagnosis**: plain-English action items

## Workflow

1. Start with `command sortit debug factor-weights` to get the overall picture.
2. Pick a low-R² issue from the `lowR2Issues` list.
3. Run `command sortit debug issue-r2 <id>` to understand why.
4. Read the `diagnosis` array for specific actions.
5. Interpret the results:

### Interpreting diagnosis

| Diagnosis | Action |
|-----------|--------|
| "Potentially misclassified tags" | The AI assigned a tag with high relevance but the tag embedding doesn't align with the issue. Consider re-enriching. |
| "Existing catalog tags close to the residual but not assigned" | Tags exist that would help but the AI missed them. Re-enrich the issue. |
| "N issues share a similar unexplained residual" | A cluster of issues have the same unexplained concept. Read their `residualNeighbors`, identify the common theme, and create a new tag for it. |
| "No existing catalog tag is close to the residual" | The issue is genuinely novel. Create a new tag that captures the concept. |

### After identifying improvements

- To re-classify: `command sortit issues re-enrich <id>`
- To re-classify multiple: `command sortit issues re-enrich <id1> <id2> <id3>`
- To see existing tags: `command sortit tags list` (or use `$sortit-tags`)
- To explore what an issue is about: `command sortit issues get <id>` (or use `$sortit-get`)

## Rules

1. Always start with `factor-weights` for the big picture before diving into individual issues.
2. Look for patterns across multiple low-R² issues — a cluster sharing the same residual direction is more actionable than a single outlier.
3. Do not suggest creating new tags unless `nearestResidualTags` shows no existing tag is close (top similarity < 0.15).
4. When recommending re-enrichment, fetch the issue first with `command sortit issues get <id>` to verify the tags look wrong.
