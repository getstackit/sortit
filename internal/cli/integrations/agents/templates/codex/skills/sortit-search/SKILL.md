---
name: sortit-search
description: Search Sortit issues from natural-language symptoms, product areas, or customer quotes. Use whenever you need to find the best existing issues before creating or editing anything. Trigger phrases include "search for issues about oauth login", "is there already a ticket for this bug", "find issues in the billing area", "any open issues matching this stack trace", "did a customer report this quote", "look up existing issues before I file one".
---

# Sortit Search

Use `command sortit issues search "<query>"` first when the user describes a problem but does not know the issue ID.

## Command

```bash
command sortit issues search "oauth login broken"
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
3. Open the strongest hit with `command sortit issues get <id>`.
4. If the user wants nearby work around a hit, use `$sortit-explore`.
