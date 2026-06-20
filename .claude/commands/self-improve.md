---
description: Review recent Sortit interactions and propose improvements that make results faster, cheaper, more accurate, or more consistent.
argument-hint: "[optional focus, e.g. search | tags | output | latency | bulk]"
allowed-tools: Read, Grep, Glob, Bash(command sortit:*), Bash(sortit:*), Skill, AskUserQuestion
---

# Self-Improve: Sortit Interaction Review

Every Sortit interaction costs the user two things: **tokens** (the model reads
the skill instructions and the CLI output) and **wall-clock** (CLI latency and
round-trips). Sortit is only worth running if it stays cheap and fast. Your job
here is to look back at how Sortit was actually used in this session and propose
changes that make it more accurate, faster, cheaper, or more consistent —
**without losing signal**.

This is the read-and-propose side. Default to suggesting, not silently applying.

## 1. Scope the evidence

Pull the most recent Sortit interactions from **this conversation's transcript**:
`sortit-*` skill invocations, `command sortit ...` / `sortit ...` CLI calls, and
any Sortit MCP tools. For each, note:

- what was asked,
- the exact command run,
- how large the output was and how much of it was actually used,
- latency if observable,
- whether it led to re-querying, filtering, or correction.

If `$ARGUMENTS` is given, focus the review on that area; otherwise cover all
recent Sortit usage. **Ground everything in what actually happened** — only cite
friction you can point to in the transcript. Do not invent problems.

## 2. Evaluate along the axes

For each interaction or recurring pattern, ask:

- **Accuracy** — did it return the right thing, or did the model have to re-query,
  re-rank, or hand-correct the result?
- **Speed** — slow command? sequential calls that could collapse into one?
  avoidable round-trips?
- **Consistency** — would the same input reliably give the same useful result? do
  two skills overlap or give conflicting guidance?
- **Cost (tokens)** — verbose output the model had to wade through? skill
  instructions longer than they need to be? context the model re-derived because
  it wasn't surfaced?

## 3. Hunt for high-leverage classes

- **Output** — trim verbose CLI output, add a compact / `--json` / `--quiet` mode,
  or surface only the fields the model actually consumes.
- **Bulk operations** — N single-item calls that should be one batched command
  (e.g. progress / close / get / link across multiple IDs at once).
- **Redundant work** — repeated searches, re-fetching results already returned,
  recall/search overlap.
- **Skill instructions** — steps that are unclear, too long, or produce
  inconsistent behavior across runs.
- **Latency** — commands that are slow per call; suggest caching, narrower
  queries, or a CLI-side fix.

## 4. Ground each suggestion in the codebase

This **is** the Sortit repo, so where a fix lives in code, point to it and read
the file before proposing the change so the suggestion is concrete and correct:

- Skill templates — `internal/cli/integrations/agents/templates/{claude,codex}/skills/sortit-*/SKILL.md`
  (Claude and Codex trees are authored independently — a skill fix usually means
  both).
- CLI — `internal/cli/...`.

A suggestion that names the file/command and the exact change is worth ten vague
ones.

## 5. Report

For each proposed improvement, give:

1. **Evidence** — the interaction(s) that exposed it.
2. **Axis** — accuracy / speed / consistency / cost.
3. **Change** — the concrete fix, with file or command.
4. **Impact × effort** — rough sizing.

Rank by impact-per-effort. Keep it to the few that genuinely matter — no padding.
If recent Sortit usage was already clean, say so plainly and stop.

## 6. Follow through (Sortit's own loop)

- For improvements worth tracking, **search first**
  (`command sortit issues search "<idea>" --status all`, or `/sortit-search`) to
  avoid duplicates, then offer to file them with `/sortit-create`.
- For uncertain or corpus-wide changes, leave them as **proposals for human
  review** — do not apply silently.
- If the user asks to apply a small, clear change now, you may — then validate per
  the repo gate (`mise run check`) before submitting.

## Rules

- Evidence over speculation — every finding ties to something that actually
  happened this session.
- Bias toward fewer, higher-impact suggestions.
- Don't add cost to fix cost — keep this analysis lean.
