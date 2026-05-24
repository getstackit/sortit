# API, CLI, and MCP

Sortit exposes the same issue system through HTTP, a local CLI, and an MCP server.

## HTTP API

The Go server listens on `http://localhost:8081` by default.

Public routes:

- `GET /`
- `GET /api/v1/health`
- `GET /api/v1/auth/github/start`
- `GET /api/v1/auth/github/callback`
- `GET /api/v1/auth/session`
- `POST /api/v1/auth/cli/login`
- `POST /api/v1/auth/cli/login/{loginID}/exchange`

Authenticated issue routes:

- `GET /api/v1/issues`
- `POST /api/v1/issues`
- `GET /api/v1/issues/search`
- `GET /api/v1/issues/{id}`
- `POST /api/v1/issues/{id}/refine`
- `POST /api/v1/issues/{id}/progress`
- `POST /api/v1/issues/{id}/close`
- `POST /api/v1/issues/{id}/reopen`
- `POST /api/v1/issues/{id}/assign`
- `POST /api/v1/issues/{id}/re-enrich`
- `POST /api/v1/issues/{id}/split`
- `GET /api/v1/issues/{id}/explore`
- batch routes under `/api/v1/issues/refine`, `/progress`, `/close`, `/assign`, and `/re-enrich`
- `POST /api/v1/issues/combine`
- `POST /api/v1/issues/link`

Other authenticated routes:

- `GET /api/v1/tags`
- `POST /api/v1/tags/merge`
- `POST /api/v1/tags/dismiss`
- `GET /api/v1/tags/dismissed`
- `GET /api/v1/people/{person}`
- `GET /api/v1/people/{person}/profile`
- `GET /api/v1/people/correlations`
- `GET /api/v1/debug/eval-tags`
- `GET /api/v1/debug/factor-weights`
- `GET /api/v1/debug/issues/{id}/r2`
- `GET /api/v1/auth/tokens`
- `POST /api/v1/auth/tokens`
- `POST /api/v1/auth/tokens/{tokenID}/revoke`

The UI uses `/api/ui`, which includes additional browser routes such as `/activity`, `/search`, `/map`, `/map/edges`, `/revision`, `/revision/stream`, and debug mutation endpoints.

## Authentication

Browser clients authenticate with the GitHub-backed session cookie.

Dedicated API clients and MCP clients use personal API tokens. After signing in:

1. Open `http://localhost:3000/settings`.
2. Create a token.
3. Store it immediately; the full token is shown once.

Send the token as:

```text
Authorization: Bearer sortit_...
```

## CLI

Authenticate the CLI:

```bash
go run ./apps/cli auth login
```

The CLI opens the browser, uses the current Sortit session to mint a personal API token, and stores local CLI config.

Common commands:

```bash
go run ./apps/cli issues create "Safari export fails after tapping Share twice"
go run ./apps/cli issues get issue-000001
go run ./apps/cli issues search "export failure"
go run ./apps/cli issues refine issue-000001 --raw "Customer confirmed this also happens on iOS 18."
go run ./apps/cli issues progress issue-000001 --raw "Added regression coverage."
go run ./apps/cli issues close issue-000001 --reason fixed
go run ./apps/cli issues assign issue-000001 --assigned-to "Ada"
go run ./apps/cli issues re-enrich issue-000001
go run ./apps/cli issues explore issue-000001
go run ./apps/cli tags list
go run ./apps/cli people profile "Ada"
go run ./apps/cli people correlations
go run ./apps/cli mine
```

The CLI defaults to `http://localhost:8081/api/v1` unless configured otherwise. It also accepts `--api-url`, `--token`, and `--config`.

## MCP

Sortit's MCP endpoint is:

```text
http://localhost:8081/mcp
```

It requires bearer-token auth. Browser session cookies are not used for `/mcp`.

Codex configuration:

```bash
export SORTIT_MCP_TOKEN='replace-with-your-token'
codex mcp remove sortit
codex mcp add sortit --url http://localhost:8081/mcp --bearer-token-env-var SORTIT_MCP_TOKEN
codex mcp get sortit
```

Claude Code configuration:

```bash
claude mcp remove -s local sortit
claude mcp add --scope local --transport http sortit http://localhost:8081/mcp \
  --header "Authorization: Bearer $SORTIT_MCP_TOKEN"
claude mcp get sortit
```

Available MCP tools:

- `create_issue`
- `search_issues`
- `get_issue`
- `list_tags`
- `refine_issues`
- `progress_issues`
- `close_issues`
- `assign_issues`
- `split_issue`
- `combine_issues`
- `link_issues`
- `explore_issue`
- `get_person_profile`
- `work_correlations`
