---
name: sortit-next
description: Find the next Sortit issue for the current user by checking assigned work first, then matching open issues to that person's tag profile when nothing is currently assigned.
version: {{VERSION}}
---

# Sortit Next

Use this skill when the user wants a starter issue, their next task, or a personalized issue recommendation.

## Primary Flow

Start with assigned work:

```bash
command sortit mine
```

If `sortit mine` returns open issues:

1. Treat the first result as the current queue head unless the user asked for a different ordering.
2. Fetch it for full context:

```bash
command sortit issues get <issue-id>
```

3. If the user wants adjacent work or a broader thread, explore it:

```bash
command sortit issues explore <issue-id>
```

## Fallback Flow

If `sortit mine` returns no open issues, infer a personalized match from the current user's profile.

1. Resolve the current user's actor name:

```bash
command sortit auth status
```

2. Fetch that person's profile. Prefer `displayName`; if empty, fall back to `login`.

```bash
command sortit people profile "<person>" --status all
```

3. Build a search query from the strongest profile tags and search open issues:

```bash
command sortit issues search "auth onboarding ui" --status open --limit 10
```

4. Inspect the strongest hits with `get` and prefer issues that are unassigned.
5. If one hit anchors a broader thread, use `explore` and return the best opportunity around it.

## Rules

1. Assigned work wins over inferred recommendations.
2. Use the current authenticated user, not a guessed name.
3. When recommending fallback issues, explain why they match the person's tag profile.
4. Prefer unassigned open issues for fallback recommendations.
5. If all strong matches are already assigned, say so explicitly and return the closest matches anyway.
