---
name: sortit-progress
description: Add progress updates to one or more Sortit issues without changing the canonical issue summary. Use when the user wants to record work done on an issue. Trigger phrases include "log progress on issue-000123", "add a progress update", "note what I did on this issue", "record that I shipped the fix", "append an update to these issues".
---

# Sortit Progress

Use `command sortit issues progress <id> [id...] --raw "<text>"` for progress logs. This records work done without modifying the canonical issue summary.

## Command

```bash
command sortit issues progress issue-000123 --raw "Shipped API fix to staging and verified the callback path."
```

Pass multiple IDs to log the same progress on several issues:

```bash
command sortit issues progress issue-000123 issue-000124 --raw "Rolled out the shared retry helper across both flows."
```

Useful flags:
- `--raw <text>` required
- `--created-by <name>`

## Related

- To update the canonical issue description with new shared context, use `$sortit-refine`.
- To find nearby or related work, use `$sortit-explore`.
