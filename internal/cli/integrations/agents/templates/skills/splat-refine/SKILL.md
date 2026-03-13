---
name: splat-refine
description: Append shared discussion or refinement context to one or more Splat issues. Use when new information should update the canonical issue description.
version: {{VERSION}}
---

# Splat Refine

Use `command splat issues refine <id> [id...] --raw "<text>"` to update issue understanding.

## Command

```bash
command splat issues refine issue-000123 --raw "Customer reports this also fails for invited users."
```

Useful flags:
- `--raw <text>` required
- `--created-by <name>`

## Rules

1. Use refine for canonical updates, not work log entries.
2. Use `splat-progress` instead when the new text describes work completed.
