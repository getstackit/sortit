---
name: sortit-search
description: Search Sortit issues from natural-language symptoms, product areas, or customer quotes. Use when you need to find the best existing issues before creating or editing anything.
allowed-tools: Bash(command sortit:*), Bash(sortit:*), Skill, AskUserQuestion
argument-hint: "<query>"
version: dev
---

# Sortit Search

Use `command sortit issues search "<query>"` first when the user describes a problem but does not know the issue ID.

## Command

```bash
command sortit issues search "$ARGUMENTS"
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
4. If the user wants nearby work around a hit, hand off to the **sortit-explore** skill (Skill tool, or `/sortit-explore <id>`).
5. Recall durable decisions and constraints too — searching issues finds open work, but a prior decision may already settle the question. Hand off to the **sortit-recall** skill (Skill tool, or `/sortit-recall <query>`). MCP `search_issues` also returns related memories alongside its results.

After presenting hits, use AskUserQuestion to offer the next step: open a specific hit (`command sortit issues get <id>`), explore nearby work (sortit-explore), recall related memories (sortit-recall), or refine the search.
