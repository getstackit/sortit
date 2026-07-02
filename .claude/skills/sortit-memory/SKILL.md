---
name: sortit-memory
description: Create, inspect, or review durable Sortit memories. Use when a coding, review, planning, or debugging session produces lasting knowledge: decisions, lessons, constraints, patterns, references, or "future agents should know" context.
allowed-tools: Bash(command sortit:*), Bash(sortit:*), Read, Grep, Glob, Skill
version: dev
---

# Sortit Memory

Memories are permanent knowledge artifacts. Use them for knowledge that should
outlive an issue or chat: decisions, lessons, constraints, patterns, and
references.

## Recall Before You Decide

Memory is a two-way loop. Before deciding or implementing, **recall** the durable
knowledge that bears on the task — prior decisions and constraints should guide
the work, not be rediscovered or contradicted. Hand off to the **sortit-recall**
skill (Skill tool, or `/sortit-recall <query>`), or recall directly:

```bash
command sortit memory search "how does Safari PDF export work"
```

## When To Create A Memory

Create a memory when the session contains durable phrasing such as:

- "we decided..."
- "remember that..."
- "future agents should know..."
- "the lesson is..."
- "the constraint is..."
- "use this pattern..."
- "this reference explains..."

Do not create a memory for ordinary task progress, transient debugging notes, or
uncertain speculation. For work done against an issue, hand off to the
**sortit-progress** skill (Skill tool, or `/sortit-progress <id>`).

## Commands

Create a manual memory:

```bash
command sortit memory create \
  --title "Safari export uses the print pipeline" \
  --kind decision \
  --anchor-tag export \
  --source-issue issue-000123 \
  "Use the print pipeline for Safari PDF export; direct canvas export loses pagination metadata."
```

List and inspect memories:

```bash
command sortit memory list --status active
command sortit memory get mem-000123
```

Synthesize proposal drafts from the corpus for human review:

```bash
command sortit memory proposals synthesize
command sortit memory proposals list
```

## Memory Kinds

- `decision` - a choice the team should follow until superseded.
- `lesson` - something learned from implementation, debugging, incidents, or review.
- `constraint` - a rule, invariant, limitation, or compatibility requirement.
- `pattern` - a reusable implementation or workflow pattern.
- `reference` - stable context worth retrieving later.
- `concept` - the canonical profile of a single noun (a subsystem, component, or
  domain concept). Unlike the other kinds (which are statements), a concept's
  subject *is* a tag: pass `--kind concept --subject-tag <tag>`. It is bound 1:1
  to that tag and supersedes any prior concept for the same tag.

## Rules

1. Prefer a short, specific title that can work as a map landmark.
2. Include `--source-issue` when a memory came from issue work.
3. Use `--anchor-tag` for high-value tags that locate the memory.
4. For a `concept`, `--subject-tag` is required — it is the noun the concept defines.
5. Create direct memories only for clear, durable knowledge.
5. When the knowledge is inferred from a cluster or confidence is low, synthesize
   or list proposals and leave human review to decide.
