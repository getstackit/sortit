---
name: sortit-assign
description: Assign or unassign Sortit issues, setting or clearing ownership. Use when ownership needs to be set or cleared. Trigger phrases include "assign issue-000123 to Jon", "give this issue to me", "unassign that bug", "who owns this issue", "change the owner of these issues", "clear the assignee".
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

To pick the right work for someone before assigning, use `$sortit-next`. To find candidate issues first, use `$sortit-search`.
