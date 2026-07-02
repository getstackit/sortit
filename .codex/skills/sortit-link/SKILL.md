---
name: sortit-link
description: "Link two Sortit issues with an explicit relationship, keeping them separate while recording how they relate. Use when issues should stay distinct but their connection matters. Trigger phrases include \"link these issues\", \"mark issue-000101 as related to issue-000123\", \"this is a duplicate of that issue\", \"set issue X as parent of issue Y\", \"connect these two issues\", \"this issue is derived from that one\"."
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

## Related skills

- Use `$sortit-explore` to find related or adjacent issues before linking.
- Use `$sortit-combine` when issues are duplicates that should be consolidated into one canonical issue rather than just linked.

<!-- sortit-version: dev -->
