---
name: sortit-diagnose
description: "Diagnose the factor model quality and identify issues where the tag taxonomy is failing. Trigger phrases include \"improve the tags\", \"what's our R²\", \"why are these issues badly tagged\", \"find issues that need re-enrichment\", \"diagnose the factor model\", \"which tags are misclassified\", and \"find clusters of poorly explained issues\"."
---

# Sortit Diagnose

Inspect factor model quality and identify actionable improvements to the tag taxonomy.

## Commands

### Corpus-wide diagnostics

Two complementary views — start with drift for mis-tagging.

**Tag health — drift (primary):**

```bash
command sortit debug tag-health
```

Open issues whose assigned tags disagree with their embedding geometry, worst-first:
- **highDriftIssues[].driftCosine**: agreement between the embedding-derived loadings and the AI's tags (1.0 = agree even under shrinkage, near-0/negative = genuine disagreement)
- **highDriftIssues[].spuriousTags**: tags the AI over-claimed — the embedding doesn't support them
- **highDriftIssues[].missingTags**: catalog tags the embedding wants but the AI didn't assign
- **driftThreshold / lambdaUnscored**: the cutoff and penalty used (tunable hyperparameters)

**Factor weights — R² (uncovered concepts):**

```bash
command sortit debug factor-weights
```

- **aggregateR2**: how much variance the tag structure explains overall (higher = better taxonomy)
- **factorWeight / residualWeight**: data-driven blend weights
- **lowR2Issues**: issues poorly explained by tags — finds concepts *no* catalog tag covers (drift can't flag these; it only compares against the tags that exist)

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

1. Start with `command sortit debug tag-health` — issues whose tags are demonstrably wrong (spurious) or incomplete (missing), with the offending tags named.
2. Act on the attribution per issue (table below): re-enrich to drop spurious tags and pick up missing ones.
3. For taxonomy-level gaps — concepts *no* catalog tag covers — switch to `factor-weights`, pick a low-R² issue, run `issue-r2 <id>`, read its `residualNeighbors`, and create a new tag for the shared concept.

### Interpreting tag-health (drift)

| Signal | Action |
|--------|--------|
| `spuriousTags` non-empty | The AI over-claimed these tags — the embedding doesn't support them. Re-enrich the issue; remove the tag if it is clearly wrong. |
| `missingTags` non-empty | Catalog tags the embedding supports but the AI didn't assign. Re-enrich to pick them up. |
| Low `driftCosine` but no dominant tag | Tagging diverges without a single culprit — read the issue and review its tags manually. |

### Interpreting issue-r2 (uncovered concepts)

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

1. Start with `tag-health` for mis-tagging (wrong/missing tags on the existing taxonomy); use `factor-weights` + `issue-r2` for uncovered concepts (no catalog tag fits).
2. Trust the drift attribution: `spuriousTags` / `missingTags` name the specific tags to fix — act on them per issue before reaching for new tags.
3. Do not suggest creating a new tag unless drift shows no `missingTags` and `issue-r2`'s `nearestResidualTags` shows no existing tag is close (top similarity < 0.15).
4. When recommending re-enrichment, fetch the issue first with `command sortit issues get <id>` to verify the tags look wrong.

<!-- sortit-version: dev -->
