---
name: sortit-combine
description: Combine multiple Sortit issues into one canonical issue. Use when several issues are duplicates or should be consolidated into a single tracked thread.
allowed-tools: Bash(command sortit:*), Bash(sortit:*)
argument-hint: "<id> <id> [id...]"
version: {{VERSION}}
---

# Sortit Combine

Synthesize a canonical issue from duplicates and close the sources.

## Command

```bash
command sortit issues combine $ARGUMENTS
```

Example:

```bash
command sortit issues combine issue-000101 issue-000123 --note "Same OAuth callback failure mode."
```

## Flags

- `--created-by <name>`
- `--note <text>`
