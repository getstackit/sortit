---
name: sortit-create
description: Create new Sortit issues through the CLI. Use when the user wants to record a bug, feature idea, stack trace, or customer quote as a new issue.
version: {{VERSION}}
---

# Sortit Create

Use `command sortit issues create "<raw>"` to create a new issue from raw text.

## Command

```bash
command sortit issues create "OAuth callback returns 500 when state is missing"
```

Useful flags:
- `--tag <tag>`
- `--created-by <name>`

## Rules

1. Search first if there is a reasonable chance the issue already exists.
2. Preserve the user's raw report language unless shortening is necessary.
3. Add tags only when they are clearly justified by the report.
