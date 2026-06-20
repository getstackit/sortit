---
name: sortit-split
description: Split a Sortit issue into child issues. Use when one issue contains multiple distinct work items that should be tracked separately. Trigger phrases include "split this issue", "break issue-000123 into separate issues", "this ticket has two unrelated tasks, split it out", "carve these into child issues", and "turn this into subtasks".
---

# Sortit Split

Use `command sortit issues split <id>` with repeated `--child` flags.

## Command

```bash
command sortit issues split issue-000123 \
  --child "Handle missing OAuth state" \
  --child "Add regression test for callback validation"
```

Useful flags:
- `--child <raw>` required, repeatable
- `--child-tag <tag>`
- `--created-by <name>`
- `--note <text>`
- `--close-source`
