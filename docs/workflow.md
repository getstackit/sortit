# Workflow

This guide describes how people are expected to *work* with Sortit — the day-to-day loop, not the setup or the internals. For where each operation lives as an API, command, or tool, see [API, CLI, and MCP](./api-cli-mcp.md). For how the scoring behind these steps works, see [Scoring, Search, and Map](./scoring-search-map.md).

## The core idea

Sortit is an issue tracker for freeform input. You don't fill in forms, pick a category, or set priority by hand. You **paste text** — a bug report, a feature idea, a stack trace, a customer quote, a working note — and the system enriches it: it canonicalizes the text, scores tag relevance, generates an embedding, and places the issue on a shared map.

The work, then, is not *data entry*. It's a loop of **capturing** raw signal, **finding** what already exists, **curating** the graph so duplicates collapse and relationships are explicit, and **moving issues forward** with refinements, progress, and ownership.

## Three surfaces, one system

Every step below can be done from any of three places, against the same backend:

- **Web app** (`http://localhost:3000`) — the visual surface: search, the map, issue detail, people, tags, settings.
- **CLI** (`go run ./apps/cli ...`) — scripted and terminal-first work.
- **MCP / agents** — Sortit's MCP tools and the `sortit-*` agent skills, so an assistant can capture and curate issues in the flow of other work.

Pick whichever fits the moment. The verbs are the same everywhere.

## The loop

```
              ┌─────────► search ──────────┐
              │                            │
   capture ───┤                            ├──► curate ──► move forward ──► resolve
              │   (find before you add)    │   (combine /   (refine /        (close /
              └────────────────────────────┘    link /       progress /       reopen)
                                                 split)        assign / next)
```

### 1. Search before you capture

The first instinct should be to **search**, not create. Sortit search takes natural-language symptoms, product areas, or customer quotes — not exact keywords — and ranks by a blend of semantic similarity, tag-factor structure, freshness, and activity. A symptom you're about to file has often already been filed in different words.

```bash
go run ./apps/cli issues search "export fails after tapping share twice"
```

If a strong match exists, refine or add progress to it instead of creating a near-duplicate. If nothing fits, capture.

### 2. Capture

Drop the raw text in. Don't pre-format, pre-categorize, or summarize — the canonicalizer and tagger do that. The more faithful the raw signal (real error text, the customer's actual words), the better the enrichment.

```bash
go run ./apps/cli issues create "Safari export fails after tapping Share twice"
```

Behind the scenes the backend embeds the text, builds a retrieval-first tag candidate set, scores tag relevance, verifies those tags, and persists the issue plus its append-only history. You don't wait on or manage any of that — the issue is searchable and on the map once enrichment lands.

### 3. Curate the graph

Sortit's value compounds when the issue graph reflects reality. Three explicit moves keep it honest:

- **Combine** when several issues are genuinely the *same* thing. They consolidate into one canonical issue; the others point at it. Use this for duplicates you want collapsed.
- **Link** when issues are distinct but *related* — and should stay separate. Relationships are explicit (`related_to`, `duplicate_of`, `parent_of` / `child_of`, `derived_from`, `merged_into`), so the connection is recorded without losing either issue.
- **Split** when one issue is secretly several work items. It becomes a parent with child issues that can be tracked independently.

To discover candidates for any of these, **explore** from a known issue — it surfaces duplicates, adjacent work, and similar open issues around it:

```bash
go run ./apps/cli issues explore issue-000001
```

The **map** is the visual counterpart: position comes from tag/factor structure (issues near each other share tag relevance), while edges come from text-embedding similarity. Two issues can sit far apart but be edge-connected (different tags, similar wording), or sit close with no edge (similar structure, actually different topics). Both are signals worth curating on.

### 4. Move issues forward

As an issue lives, new information arrives. Two distinct verbs keep the record clean:

- **Refine** when the new information changes *what the issue is*. A refinement updates the canonical description and re-enriches (re-tags) the issue. Use it for confirmations, new reproduction details, expanded scope.

  ```bash
  go run ./apps/cli issues refine issue-000001 --raw "Customer confirmed this also happens on iOS 18."
  ```

- **Progress** when you're recording *work done* without changing what the issue fundamentally is. Progress posts add to the discussion and signal activity (which feeds velocity), but they leave the canonical summary alone.

  ```bash
  go run ./apps/cli issues progress issue-000001 --raw "Added regression coverage."
  ```

The rule of thumb: **refine changes the issue, progress reports against it.**

If tagging drifts after edits — or the tag catalog itself has improved — **re-enrich** to recompute tags and embedding without otherwise touching the issue:

```bash
go run ./apps/cli issues re-enrich issue-000001
```

### 5. Ownership and "what's next"

- **Assign** (or unassign) to set ownership.

  ```bash
  go run ./apps/cli issues assign issue-000001 --assigned-to "Ada"
  ```

- **Next** answers "what should I work on?" It checks your assigned work first; when nothing is assigned, it matches open issues against your tag profile — the kinds of issues you tend to work on. So the queue is personalized rather than a flat list.

- **Mine** lists the issues currently on your plate.

  ```bash
  go run ./apps/cli mine
  ```

### 6. Resolve

**Close** with a reason when an issue is fixed, obsolete, or intentionally dropped; **reopen** if it comes back. Resolution state feeds visibility and ranking — duplicates and merged issues can be hidden behind their canonical target, so closing and combining keep search surfaces clean.

```bash
go run ./apps/cli issues close issue-000001 --reason fixed
```

## Working with the whole corpus

Beyond the single-issue loop, Sortit treats your issues as a body of knowledge:

- **People analytics** — each person has a tag profile derived from the issues they touch. Profiles drive `next`'s recommendations, and correlations show where two people's work overlaps.

  ```bash
  go run ./apps/cli people profile "Ada"
  go run ./apps/cli people correlations
  ```

- **Tags** — the tag catalog is the vocabulary the whole system reasons in. Inspect it before filtering or to understand how issues are being classified.

  ```bash
  go run ./apps/cli tags list
  ```

- **Tag quality / diagnosis** — because tags drive search, the map, and people profiles, the taxonomy is worth maintaining. Diagnose surfaces where the factor model is weak (low R²) and which issues need re-enrichment; tags can also be **merged** (collapse synonyms) or **dismissed** (suppress noise). Healthy tags make every other surface sharper.

## A typical session

1. New report lands. **Search** it — is this already tracked?
2. Match found → **refine** it (new info) or add **progress** (work done). No match → **create**.
3. Periodically, **explore** or scan the **map** for clusters → **combine** duplicates, **link** the merely-related, **split** the overloaded.
4. **Assign** owners; each person uses **next** to pull personalized work.
5. **Close** what's done; **reopen** what regresses.
6. Occasionally, **diagnose** tag health and **re-enrich** or **merge** to keep the model honest.

The discipline that makes Sortit pay off is small and repeatable: search before you create, separate *refine* from *progress*, and curate relationships explicitly rather than letting duplicates pile up.
