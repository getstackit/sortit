---
name: sortit-close
description: Close one or more Sortit issues when they are resolved, obsolete, or intentionally closed out. Trigger phrases include "close this issue", "close issue-000123", "mark these issues as done", "this is resolved, close it", "shut these tickets down", and "close out this duplicate".
---

# Sortit Close

Use `command sortit issues close <id> [id...]` to close issues.

## Command

```bash
command sortit issues close issue-000123 --closed-by "Jon"
```

Close multiple issues in one call by listing several IDs:

```bash
command sortit issues close issue-000123 issue-000124 --closed-by "Jon"
```

Useful flags:
- `--closed-by <name>`

## Related

- Use `$sortit-combine` instead when several issues are duplicates that should consolidate into one canonical issue.
- Use `$sortit-explore` to check for nearby or related work before closing.
