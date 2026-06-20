---
name: sortit-split
description: Split a Sortit issue into child issues. Use when one issue contains multiple distinct work items that should be tracked separately.
allowed-tools: Bash(command sortit:*), Bash(sortit:*)
argument-hint: "<issue-id>"
version: {{VERSION}}
---

# Sortit Split

Use `command sortit issues split <id>` with repeated `--child` flags.

## Command

```bash
command sortit issues split "$ARGUMENTS" \
  --child "Handle missing OAuth state" \
  --child "Add regression test for callback validation"
```

Useful flags:
- `--child <raw>` required, repeatable
- `--child-tag <tag>`
- `--created-by <name>`
- `--note <text>`
- `--close-source`
