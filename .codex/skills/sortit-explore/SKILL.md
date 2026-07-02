---
name: sortit-explore
description: "Explore related Sortit issues from a known issue ID, surfacing duplicates, adjacent work, and similar open issues. Trigger phrases include \"show me issues related to issue-000123\", \"find duplicates of this issue\", \"what else is adjacent to this\", \"explore similar open issues around issue-X\", and \"what work is near this issue\"."
---

# Sortit Explore

Use `command sortit issues explore <issue-id>` after you already have a concrete issue ID.

## Command

```bash
command sortit issues explore issue-000123
```

Useful flags:
- `--limit <n>`

## Tips

- If an issue's tags look wrong, re-classify it with `command sortit issues re-enrich <id>` instead of creating a fake refinement post.
- Check R² diagnostics with `command sortit debug issue-r2 <id>` to understand how well tags explain a specific issue.

## Rules

1. Use `$sortit-search` if the issue ID is not known yet.
2. Fetch the issue first with `command sortit issues get <id>` (or `$sortit-get`) if you need its full context before summarizing related work.

<!-- sortit-version: dev -->
