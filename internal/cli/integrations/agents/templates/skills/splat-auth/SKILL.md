---
name: splat-auth
description: Authenticate the Splat CLI. Use when CLI calls fail due to missing or stale credentials.
version: {{VERSION}}
---

# Splat Auth

Use these commands to establish or inspect CLI authentication:

```bash
command splat auth status
command splat auth login --web
command splat auth logout
```

If API calls fail with auth errors, check `status` first and then re-run `login --web`.
