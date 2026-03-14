---
name: splat-search
description: Search Splat issues from natural-language symptoms, product areas, or customer quotes. Use when you need to find the best existing issues before creating or editing anything.
version: {{VERSION}}
---

# Splat Search

Use `command splat issues search "<query>"` first when the user describes a problem but does not know the issue ID.

## Command

```bash
command splat issues search "oauth login broken"
```

Useful flags:
- `--status open|closed|all`
- `--assigned-to <name>`
- `--tag <tag>`
- `--limit <n>`
- `--offset <n>`
- `--sort-by relevance|created_at`

## Follow Through

1. Start with the user's own wording.
2. If results are broad, add tags, assignee, or `--status all`.
3. Open the strongest hit with `command splat issues get <id>`.
4. If the user wants nearby work around a hit, switch to the `splat-explore` skill.
