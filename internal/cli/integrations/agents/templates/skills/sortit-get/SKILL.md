---
name: sortit-get
description: Fetch a Sortit issue by ID. Use when the user references a known issue and you need the canonical details.
version: {{VERSION}}
---

# Sortit Get

Use `command sortit issues get <issue-id>` to retrieve a specific issue.

## Command

```bash
command sortit issues get issue-000123
```

## Rules

1. Prefer this over search when the user already knows the exact ID.
2. Use `sortit-explore` after `get` if the user wants related work.
