---
name: sortit-create
description: Create a new Sortit issue from raw text. Use when the user wants to record a bug, feature idea, stack trace, or customer quote as a new issue.
allowed-tools: Bash(command sortit:*), Bash(sortit:*), Skill
argument-hint: "<raw issue text>"
version: dev
---

# Sortit Create

Create a new issue from raw text.

## Command

```bash
command sortit issues create "$ARGUMENTS"
```

Example:

```bash
command sortit issues create "OAuth callback returns 500 when state is missing"
```

Useful flags:
- `--tag <tag>`
- `--created-by <name>`

## Rules

1. Search first if there is a reasonable chance the issue already exists. Hand off to the **sortit-search** skill (Skill tool, or `/sortit-search <query>`).
2. Preserve the user's raw report language unless shortening is necessary.
3. Add tags only when they are clearly justified by the report.
