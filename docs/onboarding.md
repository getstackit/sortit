# Onboarding a New Project: Bootstrapping the Tag Vocabulary

When a new project adopts Sortit, the single most important thing to get right is
the **tag vocabulary** — and it is the one thing that does *not* take care of
itself. This doc explains the cold-start problem and the bootstrap flow that
avoids it.

## The cold-start problem

A fresh Sortit instance seeds its catalog from `DefaultTags()`
(`internal/issues/store.go`) — ~17 deliberately generic, cross-project tags:
`bug`, `backend`, `feature`, `improvement`, `ui`, `ux`, `frontend`, … They exist
so the very first issue has *something* to match against.

But generic seeds become a trap. Enrichment builds each issue's candidate tag set
by retrieving tags already in the catalog (by embedding similarity) plus a fixed
generic anchor set, and the tagger is told to express an issue with existing tags
before coining a new one. So:

- Project-specific tags (`ridge-regression`, `memory-synthesis`) are never in the
  candidate set, because they were never seeded and have no embedding to retrieve.
- The tagger describes every issue by its generic surface (`backend`,
  `improvement`) — which it *can* always do.
- Those generic tags get reinforced, and the equilibrium locks in.

The cost is measurable. On a real corpus left to its own devices, the factor
model's **aggregate R² sits near 0.04** — the tag structure explains ~4% of the
corpus, and the model puts 95% of its weight on raw text because the tags carry
almost no signal. See [scoring-search-map.md](./scoring-search-map.md) for what
R² is and why it matters. A generic taxonomy makes search, the map, regions, and
synthesis all weaker than they should be.

## The principle: seed the vocabulary first

The fix is to **establish a project-specific vocabulary before the corpus fills
with generic tags** — and the right unit for that vocabulary is the
[concept memory](./data-model.md#memory-and-curation-state). A concept is the
canonical profile of a single noun (a subsystem, algorithm, or domain concept),
bound 1:1 to a tag. Authoring a concept **seeds its subject tag into the catalog**
(with an embedding derived from the concept body), so the curated noun immediately
becomes a retrieval candidate that future issues can be tagged with.

Concepts are therefore not just durable knowledge — they are how a project
*teaches Sortit its own language*.

## The bootstrap flow

1. **Derive the core concepts from the project itself**, not from issue text. The
   distinctive subsystems, algorithms, and domain nouns the codebase is built
   around — the things a domain expert would name to explain what makes the
   project distinctive. Aim for 10–20 to start; breadth matters (each concept
   only covers the issues that are about it).

2. **Author them as concept memories** (`kind=concept`, `subject_tag=<tag>`), via
   the web composer, `create_memory` (MCP), or `POST /memories`. Each one seeds
   its subject tag into the catalog and embeds it. Give each a real one-to-three
   sentence body using the vocabulary that appears in issues — the body becomes
   the tag's description, which is what gets embedded for retrieval.

3. **Re-enrich the corpus** (`POST /issues/re-enrich`) so existing issues are
   re-tagged against the now-richer vocabulary. New issues pick up the concepts
   automatically as they are created.

4. **Diagnose and iterate** — `sortit debug factor-weights` for the aggregate
   picture, `sortit debug issue-r2 <id>` for per-issue residuals pointing at
   concepts you haven't seeded yet. The residual literally tells you what nouns
   are missing.

This flow has been validated on a live corpus: seeding ~10 architectural concepts
and re-enriching took concept-tag adoption from **0% to ~29% of issues**, with the
seeded tags genuinely aligning with the issue embeddings. Tagging quality is the
leading indicator and responds immediately; aggregate R² is the lagging,
breadth-bound indicator that climbs as coverage and taxonomy cleanliness improve.

## What's required to make this turnkey

The core machinery already exists: `EnsureStoredTags` embeds and persists tags,
concept creation seeds the catalog, and re-enrichment re-tags. The gaps are in the
*intake surface* — today bootstrapping is a manual, one-concept-at-a-time flow.

| Capability | State today | Gap to close |
|---|---|---|
| Concept → catalog seeding | **Built** (`memories.Service.seedConceptTag`) | — |
| Embed seeded tags | **Built** (`CatalogService.EnsureStoredTags`) | — |
| Re-enrich corpus | **Built** (`POST /issues/re-enrich`) | — |
| Propose concepts from the repo | Manual (a human/agent reads the codebase) | A guided step — an agent skill or `sortit bootstrap` command that reads the repo/docs and proposes a ranked concept list for human approval |
| Bulk concept/tag import | One-at-a-time create | A batch import endpoint + a manifest format (e.g. `concepts.toml`) so a project's vocabulary is reproducible and version-controlled |
| Teach agents the project vocabulary | Generic instructions only | `sortit agent install` could install a project-vocabulary block (the concept catalog) so agents tag and search in the project's language from the start |

### Recommended next step

A **`sortit bootstrap`** flow that: (1) reads the repository and proposes core
concepts (an LLM pass over the package structure + docs, much like the manual
derivation above), (2) shows them for human approval, (3) creates the approved
concepts (seeding the catalog), and (4) optionally re-enriches the existing
corpus. This turns the validated manual playbook into a one-command onboarding
step, and keeps the project's vocabulary an explicit, reviewable artifact rather
than an emergent accident.

Until that exists, follow the manual flow above — it is the same sequence the
`bootstrap` command would automate, and it works today.
