---
name: sortit-auth
description: Authenticate the Sortit CLI. Use when CLI calls fail due to missing or stale credentials.
version: {{VERSION}}
---

# Sortit Auth

Use these commands to establish or inspect CLI authentication:

```bash
command sortit auth status
command sortit auth login --web
command sortit auth logout
```

If API calls fail with auth errors, check `status` first and then re-run `login --web`.
