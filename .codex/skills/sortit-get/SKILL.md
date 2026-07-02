---
name: sortit-get
description: "Fetch a Sortit issue by ID and return its canonical details. Use when the user references a known issue and you need the authoritative record. Trigger phrases include \"show me issue-000123\", \"get issue ABC-42\", \"pull up that issue\", \"what does issue-000123 say\", \"fetch the details for that ticket\"."
---

# Sortit Get

Use `command sortit issues get <issue-id>` to retrieve a specific issue.

## Command

```bash
command sortit issues get issue-000123
```

## Rules

1. Prefer this over search when the user already knows the exact ID.
2. Use `$sortit-explore` after `get` if the user wants related work.

<!-- sortit-version: dev -->
