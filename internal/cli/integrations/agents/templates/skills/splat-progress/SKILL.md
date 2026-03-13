---
name: splat-progress
description: Add progress updates to one or more Splat issues. Use when the user wants to record work done without changing the canonical issue summary.
version: {{VERSION}}
---

# Splat Progress

Use `command splat issues progress <id> [id...] --raw "<text>"` for progress logs.

## Command

```bash
command splat issues progress issue-000123 --raw "Shipped API fix to staging and verified the callback path."
```

Useful flags:
- `--raw <text>` required
- `--created-by <name>`
