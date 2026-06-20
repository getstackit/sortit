# Workflow

This guide describes how people are expected to *work* with Sortit — the day-to-day loop, not the setup or the internals. For where each operation lives as an API, command, or tool, see [API, CLI, and MCP](./api-cli-mcp.md). For how the scoring behind these steps works, see [Scoring, Search, and Map](./scoring-search-map.md).

## The core idea

Sortit is an issue tracker and memory system for agentic work. You don't fill in forms, pick a category, or set priority by hand. You **paste text** — a bug report, a feature idea, a stack trace, a customer quote, a working note, an agent handoff — and the system enriches it: it canonicalizes the text, scores tag relevance, generates an embedding, and places the issue on a shared map.

The website is still important: it is where people scan the map, inspect detail pages, review proposals, and build trust in the corpus. But most day-to-day interaction should happen inside the agent loop. While an assistant is debugging, planning, reviewing, or implementing, it should use Sortit to save follow-on work, bring related items into context, record progress, and preserve durable memories that future humans and agents can reuse.

The work, then, is not *data entry*. It's a collaboration loop:

- **Capture** raw signal as issues when new work appears.
- **Retrieve** related issues and memories before deciding what to do.
- **Curate** the graph so duplicates collapse, separate items stay linked, and overloaded issues split.
- **Coordinate** humans and agents through ownership, progress, proposals, and review queues.
- **Remember** decisions, lessons, constraints, patterns, and references as durable memories.

## Three surfaces, one system

Every step below can be done from any of three places, against the same backend:

- **MCP / agents** — the primary operating surface for recurring work. Agents use Sortit's MCP tools and `sortit-*` skills to search before creating, pull related context, create follow-on issues, add progress, draft curation proposals, and synthesize memories without leaving the task loop.
- **Web app** — the human review and exploration surface: search, map, issue detail, memory detail, proposals, people, tags, settings. Use it to inspect the corpus, accept or reject proposed moves, and understand the shape of the work.
- **CLI** (`sortit ...`) — the scriptable and terminal-first surface. It backs both local automation and agent skills.

Pick whichever fits the moment. The verbs are the same everywhere, but the expected default is: agents keep the corpus warm during work; humans review, steer, and make higher-judgment calls from the website or CLI.

## The loop

```
              ┌─────────► search ──────────┐
              │                            │
   capture ───┤                            ├──► curate ──► move forward ──► remember
              │   (find before you add)    │   (combine /   (refine /        (memory /
              └────────────────────────────┘    link /       progress /       proposal)
                                                 split)        assign / next)
                                                                  │
                                                                  ▼
                                                               resolve
```

### 1. Search before you capture

The first instinct should be to **search**, not create. Sortit search takes natural-language symptoms, product areas, or customer quotes — not exact keywords — and ranks by a blend of semantic similarity, tag-factor structure, freshness, and activity. A symptom you're about to file has often already been filed in different words.

```bash
sortit issues search "export fails after tapping share twice"
```

If a strong match exists, refine or add progress to it instead of creating a near-duplicate. If nothing fits, capture.

Agents should do this automatically. Before filing follow-on work from a code review, failed test, user report, or implementation note, the agent should search the corpus, inspect any relevant memories, and bring back the strongest related context. That context belongs in the work loop before the agent decides whether to create, refine, progress, link, or split.

### 2. Capture

Drop the raw text in. Don't pre-format, pre-categorize, or summarize — the canonicalizer and tagger do that. The more faithful the raw signal (real error text, the customer's actual words), the better the enrichment.

```bash
sortit issues create "Safari export fails after tapping Share twice"
```

Behind the scenes the backend embeds the text, builds a retrieval-first tag candidate set, scores tag relevance, verifies those tags, and persists the issue plus its append-only history. You don't wait on or manage any of that — the issue is searchable and on the map once enrichment lands.

Use capture for follow-on work too. When an agent discovers a nearby defect, missing test, migration task, product question, or cleanup that should not be solved in the current pass, it should create a Sortit issue instead of leaving the thought in transient chat history.

### 3. Curate the graph

Sortit's value compounds when the issue graph reflects reality. Three explicit moves keep it honest:

- **Combine** when several issues are genuinely the *same* thing. They consolidate into one canonical issue; the others point at it. Use this for duplicates you want collapsed.
- **Link** when issues are distinct but *related* — and should stay separate. Relationships are explicit (`related_to`, `duplicate_of`, `parent_of` / `child_of`, `derived_from`, `merged_into`), so the connection is recorded without losing either issue.
- **Split** when one issue is secretly several work items. It becomes a parent with child issues that can be tracked independently.

To discover candidates for any of these, **explore** from a known issue — it surfaces duplicates, adjacent work, and similar open issues around it:

```bash
sortit issues explore issue-000001
```

The **map** is the visual counterpart: position comes from tag/factor structure (issues near each other share tag relevance), while edges come from text-embedding similarity. Two issues can sit far apart but be edge-connected (different tags, similar wording), or sit close with no edge (similar structure, actually different topics). Both are signals worth curating on.

Agents can also run librarian-style curation passes. Candidate detection finds suspicious duplicates, stale issues, enrichment problems, and quiet or redundant memories; the agent reads the actual issues, memories, and code context, then files **propose-only** curation moves. A human accepts or rejects those proposals.

```bash
sortit curation candidates duplicates
sortit curation proposals list
```

### 4. Move issues forward

As an issue lives, new information arrives. Two distinct verbs keep the record clean:

- **Refine** when the new information changes *what the issue is*. A refinement updates the canonical description and re-enriches (re-tags) the issue. Use it for confirmations, new reproduction details, expanded scope.

  ```bash
  sortit issues refine issue-000001 --raw "Customer confirmed this also happens on iOS 18."
  ```

- **Progress** when you're recording *work done* without changing what the issue fundamentally is. Progress posts add to the discussion and signal activity (which feeds velocity), but they leave the canonical summary alone.

  ```bash
  sortit issues progress issue-000001 --raw "Added regression coverage."
  ```

The rule of thumb: **refine changes the issue, progress reports against it.**

This distinction matters most with agents. An agent should add progress when it tried an approach, found a failing test, opened a PR, verified a fix, or intentionally deferred work. It should refine when it learned something that changes the issue's definition: new reproduction steps, a narrower scope, a confirmed cause, or evidence that the original report was broader than expected.

If tagging drifts after edits — or the tag catalog itself has improved — **re-enrich** to recompute tags and embedding without otherwise touching the issue:

```bash
sortit issues re-enrich issue-000001
```

### 5. Ownership and "what's next"

- **Assign** (or unassign) to set ownership.

  ```bash
  sortit issues assign issue-000001 --assigned-to "Ada"
  ```

- **Next** answers "what should I work on?" It checks your assigned work first; when nothing is assigned, it matches open issues against your tag profile — the kinds of issues you tend to work on. So the queue is personalized rather than a flat list.

- **Mine** lists the issues currently on your plate.

  ```bash
  sortit mine
  ```

For agents, `next` is the bridge between the corpus and the current work loop. It lets an assistant pull from assigned work first, then from issues that match the human or agent's tag profile. The result should not be treated as a blind queue; it is the starting context for search, exploration, related memories, and a clear handoff back to the human.

### 6. Remember what should outlive the issue

Issues track work. **Memories** track durable knowledge: decisions, lessons, constraints, patterns, and references that should remain useful after the issue closes. Memories live in the same tag and embedding space as issues, show up as map landmarks, and are retrieved during enrichment as prior-decision context.

Create a memory when the team learns something that should guide future work:

```bash
sortit memory create \
  --title "Safari export uses the print pipeline" \
  --kind decision \
  --anchor-tag export \
  --source-issue issue-000001 \
  "Use the print pipeline for Safari PDF export; direct canvas export loses pagination metadata."
```

Memories can also be synthesized from the corpus. Agents should run synthesis as part of periodic curation, then leave the proposals for human review:

```bash
sortit memory proposals synthesize
sortit memory proposals list
```

Memories are permanent by default. Supersede or archive them only when newer knowledge replaces them or they are genuinely obsolete.

### 7. Resolve

**Close** with a reason when an issue is fixed, obsolete, or intentionally dropped; **reopen** if it comes back. Resolution state feeds visibility and ranking — duplicates and merged issues can be hidden behind their canonical target, so closing and combining keep search surfaces clean.

```bash
sortit issues close issue-000001 --reason fixed
```

## Working with the whole corpus

Beyond the single-issue loop, Sortit treats your issues as a body of knowledge:

- **People analytics** — each person has a tag profile derived from the issues they touch. Profiles drive `next`'s recommendations, and correlations show where two people's work overlaps.

  ```bash
  sortit people profile "Ada"
  sortit people correlations
  ```

- **Tags** — the tag catalog is the vocabulary the whole system reasons in. Inspect it before filtering or to understand how issues are being classified.

  ```bash
  sortit tags list
  ```

- **Tag quality / diagnosis** — because tags drive search, the map, and people profiles, the taxonomy is worth maintaining. Diagnose surfaces where the factor model is weak (low R²) and which issues need re-enrichment; tags can also be **merged** (collapse synonyms) or **dismissed** (suppress noise). Healthy tags make every other surface sharper.

- **Memories** — permanent knowledge artifacts share the same retrieval space as issues. Use memory detail pages, memory lists, and map landmarks to inspect decisions, constraints, patterns, references, and lessons that should influence future work.

- **Curation proposals** — agent-drafted moves wait in a review queue. Humans accept or reject proposals to combine duplicates, close stale work, re-enrich issues, archive obsolete memories, or supersede redundant memories. This keeps agents useful without letting them silently rewrite the corpus.

## A typical session

1. A human or agent starts work. The agent **searches** Sortit, pulls related issues and memories, and uses that context before acting.
2. New information appears. Match found → **refine** it (new issue definition) or add **progress** (work done). No match → **create** a follow-on issue.
3. The agent uses **explore** to bring in adjacent work, then proposes or performs the appropriate relationship move: **combine**, **link**, or **split**.
4. People inspect the **website** to scan the map, review issue and memory detail, and accept or reject curation and memory proposals.
5. **Assign** owners; humans and agents use **next** to pull personalized work.
6. When work teaches something durable, create or accept a **memory** with source issues as provenance.
7. **Close** what's done; **reopen** what regresses.
8. Periodically, an agent runs a librarian pass: **diagnose** tag health, draft curation proposals, synthesize memory proposals, and summarize what needs human review.

The discipline that makes Sortit pay off is small and repeatable: agents search before they create, save follow-on work instead of losing it in chat, separate *refine* from *progress*, curate relationships explicitly, and turn durable lessons into memories humans can trust.
