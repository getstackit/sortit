---
name: splat-get
description: Fetch a Splat issue by ID. Use when the user references a known issue and you need the canonical details.
version: {{VERSION}}
---

# Splat Get

Use `command splat issues get <issue-id>` to retrieve a specific issue.

## Command

```bash
command splat issues get issue-000123
```

## Rules

1. Prefer this over search when the user already knows the exact ID.
2. Use `splat-explore` after `get` if the user wants related work.
