---
name: sortit-get
description: Fetch a Sortit issue by ID. Use when the user references a known issue and you need the canonical details.
allowed-tools: Bash(command sortit:*), Bash(sortit:*), Skill
argument-hint: "<issue-id>"
version: {{VERSION}}
---

# Sortit Get

Retrieve a specific issue by its exact ID with `command sortit issues get`.

## Command

```bash
command sortit issues get "$ARGUMENTS"
```

Example:

```bash
command sortit issues get issue-000123
```

## Rules

1. Prefer this over search when the user already knows the exact ID.
2. After `get`, if the user wants related work, hand off to the **sortit-explore** skill (Skill tool, or `/sortit-explore <id>`).
