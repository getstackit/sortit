---
name: sortit-refine
description: Append refinement context to one or more Sortit issues, updating the canonical description. Use when new shared discussion or information should change how an issue is understood.
allowed-tools: Bash(command sortit:*), Bash(sortit:*), Skill
argument-hint: "<id> [id...] --raw \"<text>\""
version: {{VERSION}}
---

# Sortit Refine

Use `command sortit issues refine <id> [id...] --raw "<text>"` to update issue understanding.

## Command

```bash
command sortit issues refine $ARGUMENTS
```

Example:

```bash
command sortit issues refine issue-000123 --raw "Customer reports this also fails for invited users."
```

Useful flags:
- `--raw <text>` required
- `--created-by <name>`

## Rules

1. Use refine for canonical updates, not work log entries.
2. When the new text describes work completed, hand off to the **sortit-progress** skill (Skill tool, or `/sortit-progress <id>`) instead.
