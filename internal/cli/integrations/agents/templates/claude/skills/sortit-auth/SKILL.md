---
name: sortit-auth
description: Authenticate the Sortit CLI. Use when CLI calls fail due to missing or stale credentials.
allowed-tools: Bash(command sortit:*), Bash(sortit:*)
version: {{VERSION}}
---

# Sortit Auth

## Context

- Auth status: !`command sortit auth status`

## Commands

Establish or inspect CLI authentication:

```bash
command sortit auth status
command sortit auth login --web
command sortit auth logout
```

## Rules

- If API calls fail with auth errors, check `status` first, then re-run `login --web`.
