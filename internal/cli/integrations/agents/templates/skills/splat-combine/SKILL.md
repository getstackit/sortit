---
name: splat-combine
description: Combine multiple Splat issues into a canonical issue. Use when several issues are duplicates or should be consolidated into one tracked thread.
version: {{VERSION}}
---

# Splat Combine

Use `command splat issues combine <id> <id> [id...]` to synthesize a canonical issue and close the sources.

## Command

```bash
command splat issues combine issue-000101 issue-000123 --note "Same OAuth callback failure mode."
```

Useful flags:
- `--created-by <name>`
- `--note <text>`
