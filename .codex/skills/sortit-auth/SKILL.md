---
name: sortit-auth
description: "Authenticate the Sortit CLI. Use when CLI calls fail due to missing or stale credentials, when commands return auth or 401 errors, or when you need to log in, log out, or check login state. Trigger phrases include \"sortit says I'm not authenticated\", \"log in to sortit\", \"sortit auth failing\", \"re-authenticate the CLI\", \"check my sortit login status\", \"sortit token expired\"."
---

# Sortit Auth

Use these commands to establish or inspect CLI authentication:

```bash
command sortit auth status
command sortit auth login --web
command sortit auth logout
```

If API calls fail with auth errors, check `status` first and then re-run `login --web`.

<!-- sortit-version: dev -->
