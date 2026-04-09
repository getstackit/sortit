---
name: sortit-link
description: Link two Sortit issues with an explicit relationship. Use when issues should stay separate but their relationship matters.
version: {{VERSION}}
---

# Sortit Link

Use `command sortit issues link --source <id> --target <id> --type <relationship>`.

## Command

```bash
command sortit issues link \
  --source issue-000101 \
  --target issue-000123 \
  --type related_to
```

Supported link types:
- `parent_of`
- `child_of`
- `merged_into`
- `derived_from`
- `related_to`
- `duplicate_of`

Useful flags:
- `--created-by <name>`
- `--note <text>`
