---
name: splat-explore
description: Explore related Splat issues from a known issue ID. Use when the user wants duplicates, adjacent work, or similar open issues around a specific issue.
version: {{VERSION}}
---

# Splat Explore

Use `command splat issues explore <issue-id>` after you already have a concrete issue ID.

## Command

```bash
command splat issues explore issue-000123
```

Useful flags:
- `--limit <n>`

## Tips

- If an issue's tags look wrong, re-classify it with `command splat issues re-enrich <id>` instead of creating a fake refinement post.
- Check R² diagnostics with `command splat debug issue-r2 <id>` to understand how well tags explain a specific issue.

## Rules

1. Use `splat-search` if the issue ID is not known yet.
2. Fetch the issue first with `command splat issues get <id>` if you need its full context before summarizing related work.
