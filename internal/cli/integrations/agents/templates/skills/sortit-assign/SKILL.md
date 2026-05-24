---
name: sortit-assign
description: Assign or unassign Sortit issues. Use when ownership needs to be set or cleared.
version: {{VERSION}}
---

# Sortit Assign

Use `command sortit issues assign <id> [id...] --assigned-to <name>` to assign issues.

## Command

```bash
command sortit issues assign issue-000123 --assigned-to "Jon"
```

Useful flags:
- `--assigned-to <name>`
- `--created-by <name>`

Pass an empty assignee to unassign when the caller can supply an explicit empty value.
