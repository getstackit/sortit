---
name: sortit-recall
description: Recall durable Sortit memories relevant to what you are about to do — prior decisions, constraints, lessons, and patterns — before deciding, implementing, or answering. Trigger phrases include "what do we know about ...", "have we decided how to ...", "any constraints on ...", "recall memories about ...", "check prior decisions before I change this", "is there a documented pattern for ...".
---

# Sortit Recall

Memories are the team's durable knowledge: decisions, constraints, lessons,
patterns, and references. Recall the relevant ones **before** you decide how to
implement, refactor, or answer — so prior decisions guide the work instead of
being rediscovered or contradicted. This is the read side of memory: recall
before you decide, the same way you search before you create.

## Command

```bash
command sortit memory search "how does Safari PDF export work"
```

Useful flags:
- `--limit <n>` — how many memories to return (default 5).
- `--min-similarity <0-1>` — drop weak matches. Results are ranked and carry a
  `similarity` score, so you can judge relevance.

## When To Recall

- Before implementing or refactoring an area — check for constraints and decisions.
- Before answering a "how do we..." / "why do we..." question — a memory may already settle it.
- Before filing or refining an issue — a prior decision may change what you write.
- When MCP `search_issues` already rode related memories along in its results, read
  those first; use this skill to recall directly with a more specific query.

## Follow Through

1. Phrase the query as what you are about to work on, decide, or look up.
2. Read the top hits. Treat high-similarity decisions and constraints as binding
   until superseded.
3. If a recalled memory is outdated, do not silently ignore it — supersede or
   archive it with `$sortit-memory`.
