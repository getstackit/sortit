# Development

## Prerequisites

Install toolchains and dependencies:

```bash
mise install
npm install
```

The repo pins npm to the `linked` install strategy via `.npmrc`. This keeps frontend dependencies under `node_modules/.store`, which prevents Go tooling from traversing incidental Go files in frontend dependencies.

## Local Hostname Rule

Use one hostname consistently. The default local setup uses `localhost`:

- web UI: `http://localhost:3000`
- API: `http://localhost:8081`
- GitHub callback: `http://localhost:8081/api/v1/auth/github/callback`
- MCP endpoint: `http://localhost:8081/mcp`

Do not mix `localhost` and `127.0.0.1` during auth setup. Session and OAuth state cookies are host-specific.

If you intentionally use `127.0.0.1`, update all of these together:

- `SORTIT_WEB_ORIGIN`
- `NEXT_PUBLIC_API_ORIGIN`
- `SORTIT_SERVER_CORS`
- the GitHub OAuth callback URL
- the browser URL you open
- the MCP URL configured in clients

## GitHub OAuth

The server requires GitHub OAuth credentials at startup.

Create a GitHub OAuth App with:

- Homepage URL: `http://localhost:3000`
- Authorization callback URL: `http://localhost:8081/api/v1/auth/github/callback`

Copy the app's client ID and client secret into `.env`.

## Environment

The repo loads `.env` via `mise`. A minimal local file:

```dotenv
GITHUB_CLIENT_ID=your_github_oauth_app_client_id
GITHUB_CLIENT_SECRET=your_github_oauth_app_client_secret
SORTIT_WEB_ORIGIN=http://localhost:3000
NEXT_PUBLIC_API_ORIGIN=http://localhost:8081
SORTIT_SERVER_CORS=http://localhost:3000,http://127.0.0.1:3000
SORTIT_DATABASE_URL=postgres://sortit:sortit@localhost:5432/sortit?sslmode=disable
SORTIT_TEST_DATABASE_URL=postgres://sortit:sortit@localhost:5432/sortit_test?sslmode=disable
```

Optional AI configuration:

```dotenv
AI_PROVIDER=openai
OPENAI_API_KEY=your_openai_api_key
OPENAI_TAG_MODEL=gpt-5.6-terra
OPENAI_CANONICAL_MODEL=gpt-4.1-mini
OPENAI_EMBED_MODEL=text-embedding-3-small
```

`OPENAI_TAG_MODEL` scores tags for create/search flows. `OPENAI_CANONICAL_MODEL` rewrites refine/combine discussion into canonical issue text before re-tagging. If `OPENAI_TAG_MODEL` is set but `OPENAI_CANONICAL_MODEL` is blank, canonicalization reuses the tag model.

## Run The App

```bash
mise run dev
```

This launches:

- PostgreSQL with `docker compose`
- the Go API on `http://localhost:8081`
- the Next.js app on `http://localhost:3000`

Open `http://localhost:3000` and sign in with GitHub.

Running `npm run dev` starts only the frontend. OAuth, API, token management, MCP, and persistence require the Go server.

## Useful Commands

```bash
mise run dev:stop
mise run check
mise run check:go
mise run check:web
mise run test
mise run test:fast
mise run lint
mise run compile
mise run db:backup
mise run cli:build
mise run server:build
```

Schema helpers:

```bash
mise run check:schema-drift
mise run generate:schema
```

Vulnerability checks (also run in CI):

```bash
mise run vuln       # Go (govulncheck) + npm audit
mise run vuln:go
mise run vuln:web
```

## Database Image

Local Postgres and CI Postgres are both `paradedb/paradedb:0.24.0-pg18` (PostgreSQL 18 + ParadeDB extensions). The tag is pinned in three places that must move together:

- `docker-compose.yml` (local dev)
- `.github/workflows/test.yml` (CI Postgres service)
- `internal/testpostgres/harness.go` (testcontainers image used by backend tests)

When upgrading:

1. Bump both files to the same `paradedb/paradedb:<version>-pg<major>` tag.
2. Recreate the local volume if the major Postgres version changes (`docker compose down -v`).
3. Run `mise run check:schema-drift` to confirm the migrated schema still matches the snapshot.
4. Run `mise run check` end-to-end before merging.

Avoid floating tags such as `latest` — they can silently drift the Postgres major version between local and CI.

## Troubleshooting

`github client id is required` or `github client secret is required`:

- `GITHUB_CLIENT_ID` or `GITHUB_CLIENT_SECRET` is missing or blank.

GitHub login redirects but the browser remains unauthenticated:

- Hostnames are mixed between `localhost` and `127.0.0.1`.
- `SORTIT_WEB_ORIGIN` does not match the browser origin.
- The GitHub OAuth callback URL does not match the backend host.

The web app loads but API calls return `401`:

- `NEXT_PUBLIC_API_ORIGIN` points at a different host than the one used for login.
- `SORTIT_SERVER_CORS` is missing the frontend origin.

MCP connects but tool calls return `authentication required`:

- The client is missing `Authorization: Bearer ...`.
- The token was revoked.
- The configured MCP URL uses the wrong host or port.
