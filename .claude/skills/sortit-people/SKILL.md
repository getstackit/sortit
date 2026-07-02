---
name: sortit-people
description: Inspect people analytics in Sortit — a person's issue profile or overlap between people. Use when the user wants per-person work signal or cross-person correlations.
allowed-tools: Bash(command sortit:*), Bash(sortit:*)
argument-hint: "<person name>"
version: dev
---

# Sortit People

Inspect people analytics.

## Commands

Profile one person:

```bash
command sortit people profile "$ARGUMENTS" --status all
```

Cross-person overlap:

```bash
command sortit people correlations --status all
```

Use `profile` for one person and `correlations` for cross-person overlap.
