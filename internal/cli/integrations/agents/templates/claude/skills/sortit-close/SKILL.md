---
name: sortit-close
description: Close one or more Sortit issues. Use when an issue is resolved, obsolete, or intentionally closed out.
allowed-tools: Bash(command sortit:*), Bash(sortit:*)
argument-hint: "<issue-id> [id...]"
version: {{VERSION}}
---

# Sortit Close

Close one or more issues with `command sortit issues close <id> [id...]`.

## Command

```bash
command sortit issues close $ARGUMENTS --closed-by "Jon"
```

Close multiple issues in one call:

```bash
command sortit issues close issue-000123 issue-000124 --closed-by "Jon"
```

## Flags

- `--closed-by <name>` — record who closed the issue
