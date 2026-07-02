---
name: sortit-people
description: "Inspect people analytics in Sortit — surface a single person's issue profile or the overlap between people. Trigger phrases include \"show me Jon's issue profile\", \"what does this person work on\", \"who has overlapping work\", \"find people with similar issues\", \"cross-person correlations\", and \"people analytics\"."
---

# Sortit People

Available commands:

```bash
command sortit people profile "Jon" --status all
command sortit people correlations --status all
```

Use `profile` for one person and `correlations` for cross-person overlap.

For nearby work around a specific issue, use `$sortit-explore`. To find the next issue for the current user, use `$sortit-next`.

<!-- sortit-version: dev -->
