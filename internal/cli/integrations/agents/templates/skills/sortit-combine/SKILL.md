---
name: sortit-combine
description: Combine multiple Sortit issues into a canonical issue. Use when several issues are duplicates or should be consolidated into one tracked thread.
version: {{VERSION}}
---

# Sortit Combine

Use `command sortit issues combine <id> <id> [id...]` to synthesize a canonical issue and close the sources.

## Command

```bash
command sortit issues combine issue-000101 issue-000123 --note "Same OAuth callback failure mode."
```

Useful flags:
- `--created-by <name>`
- `--note <text>`
