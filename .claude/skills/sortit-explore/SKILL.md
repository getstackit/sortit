---
name: sortit-explore
description: Explore related Sortit issues from a known issue ID — duplicates, adjacent work, or similar open issues. Use when you already have a concrete issue ID and want its neighbors.
allowed-tools: Bash(command sortit:*), Bash(sortit:*), Skill
argument-hint: "<issue-id>"
version: dev
---

# Sortit Explore

Use `command sortit issues explore <issue-id>` after you already have a concrete issue ID.

## Command

```bash
command sortit issues explore "$ARGUMENTS"
```

Useful flags:
- `--limit <n>`

## Tips

- If an issue's tags look wrong, re-classify it with `command sortit issues re-enrich "$ARGUMENTS"` instead of creating a fake refinement post.
- Check R² diagnostics with `command sortit debug issue-r2 "$ARGUMENTS"` to understand how well tags explain a specific issue.

## Rules

1. If the issue ID is not known yet, hand off to the **sortit-search** skill (Skill tool, or `/sortit-search <query>`).
2. Fetch the issue first with `command sortit issues get "$ARGUMENTS"` if you need its full context before summarizing related work.
