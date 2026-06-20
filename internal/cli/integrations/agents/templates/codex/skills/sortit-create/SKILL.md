---
name: sortit-create
description: Create new Sortit issues through the CLI from raw text. Use when the user wants to record a bug, feature idea, stack trace, or customer quote as a new issue. Trigger phrases include "create an issue for ...", "file a bug about ...", "log this feature request", "track this stack trace", "open a new sortit issue", "capture this customer quote as an issue".
---

# Sortit Create

Use `command sortit issues create "<raw>"` to create a new issue from raw text.

## Command

```bash
command sortit issues create "OAuth callback returns 500 when state is missing"
```

Useful flags:
- `--tag <tag>`
- `--created-by <name>`

## Rules

1. Search first if there is a reasonable chance the issue already exists. Use `$sortit-search` to find the best existing issues before creating anything.
2. Preserve the user's raw report language unless shortening is necessary.
3. Add tags only when they are clearly justified by the report. Use `$sortit-tags` to inspect the available tag catalog.

## Related

- `$sortit-explore` for nearby work around a known issue ID.
- `$sortit-combine` if the new report turns out to duplicate existing issues.
