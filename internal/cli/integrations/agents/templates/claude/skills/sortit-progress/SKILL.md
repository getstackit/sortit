---
name: sortit-progress
description: Add progress updates to one or more Sortit issues. Use when the user wants to record work done without changing the canonical issue summary.
allowed-tools: Bash(command sortit:*), Bash(sortit:*), Skill
argument-hint: "<issue-id> <progress text>"
version: {{VERSION}}
---

# Sortit Progress

Use `command sortit issues progress <id> [id...] --raw "<text>"` for progress logs.

## Command

```bash
command sortit issues progress issue-000123 --raw "$ARGUMENTS"
```

Useful flags:
- `--raw <text>` required
- `--created-by <name>`

## Related

- If the new information changes *what the issue is* (not just work done against it), hand off to the **sortit-refine** skill (Skill tool, or `/sortit-refine <id>`). Refine updates the canonical description and re-enriches; progress leaves it alone.
- To find nearby or related work, hand off to the **sortit-explore** skill (Skill tool, or `/sortit-explore <id>`).
