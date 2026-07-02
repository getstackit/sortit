---
name: sortit-refine
description: "Append shared discussion or refinement context to one or more Sortit issues, updating the canonical issue description. Use when new information should update what an issue is fundamentally about. Trigger phrases include \"refine issue-000123\", \"add this context to the issue\", \"update the issue description with...\", \"this bug also affects invited users — note it on the issue\", and \"append discussion notes to these issues\"."
---

# Sortit Refine

Use `command sortit issues refine <id> [id...] --raw "<text>"` to update issue understanding.

## Command

```bash
command sortit issues refine issue-000123 --raw "Customer reports this also fails for invited users."
```

Useful flags:
- `--raw <text>` required
- `--created-by <name>`

## Rules

1. Use refine for canonical updates, not work log entries.
2. Use `$sortit-progress` instead when the new text describes work completed.

<!-- sortit-version: dev -->
