---
name: sortit-assign
description: Assign or unassign Sortit issues. Use when ownership needs to be set or cleared.
allowed-tools: Bash(command sortit:*), Bash(sortit:*)
argument-hint: "<issue-id> [issue-id...] --assigned-to <name>"
version: {{VERSION}}
---

# Sortit Assign

Assign or unassign one or more issues with `command sortit issues assign <id> [id...] --assigned-to <name>`.

## Command

```bash
command sortit issues assign $ARGUMENTS
```

Example:

```bash
command sortit issues assign issue-000123 --assigned-to "Jon"
```

## Flags

- `--assigned-to <name>`
- `--created-by <name>`

## Rules

- Accepts multiple issue IDs in one call.
- Pass an empty assignee to unassign when the caller can supply an explicit empty value.
