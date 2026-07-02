---
name: sortit-combine
description: Combine multiple Sortit issues into one canonical issue. Use when several issues are duplicates or should be consolidated into a single tracked thread.
allowed-tools: Bash(command sortit:*), Bash(sortit:*), Skill
argument-hint: "<id> <id> [id...]"
version: dev
---

# Sortit Combine

Synthesize a canonical issue from duplicates and close the sources.

## Command

```bash
command sortit issues combine $ARGUMENTS
```

Example:

```bash
command sortit issues combine issue-000101 issue-000123 --note "Same OAuth callback failure mode."
```

## Flags

- `--created-by <name>`
- `--note <text>`

## Related skills

- Hand off to the **sortit-search** skill (Skill tool, or `/sortit-search <query>`) to surface the best existing issues from symptoms or quotes first.
- Hand off to the **sortit-explore** skill (Skill tool, or `/sortit-explore <id>`) to find duplicates and adjacent work around a known issue before combining.
- Hand off to the **sortit-link** skill (Skill tool, or `/sortit-link <source> <target> <type>`) when issues are distinct but related and should stay separate.
