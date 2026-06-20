---
name: sortit-combine
description: Combine multiple Sortit issues into one canonical issue and close the sources. Use when several issues are duplicates or should be consolidated into a single tracked thread. Trigger phrases include "combine these issues", "merge issue-000101 and issue-000123", "these are duplicates, consolidate them", "roll these issues into one", "dedupe these tickets", and "make one canonical issue from these".
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

## Related skills

- Use `$sortit-explore` to find duplicates and adjacent work around a known issue before combining.
- Use `$sortit-search` to surface the best existing issues from symptoms or quotes first.
- Use `$sortit-link` when issues should stay separate but their relationship matters.
