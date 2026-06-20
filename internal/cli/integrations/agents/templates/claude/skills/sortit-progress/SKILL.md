---
name: sortit-progress
description: Add progress updates to one or more Sortit issues. Use when the user wants to record work done without changing the canonical issue summary.
allowed-tools: Bash(command sortit:*), Bash(sortit:*)
argument-hint: "<issue-id> <progress text>"
version: {{VERSION}}
---

# Sortit Progress

Use `command sortit issues progress <id> [id...] --raw "<text>"` for progress logs.

## Command

```bash
command sortit issues progress issue-000123 --raw "$ARGUMENTS"
```

Useful flags:
- `--raw <text>` required
- `--created-by <name>`
