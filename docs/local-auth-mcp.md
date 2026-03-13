# Local GitHub Auth and MCP Setup

This guide covers the local developer setup for:

- GitHub OAuth login for the web app
- required environment variables
- `localhost` vs `127.0.0.1` hostname consistency
- local startup
- personal API token creation
- Codex and Claude MCP configuration

## 1. Pick One Hostname and Stick to It

The easiest local setup is to use `localhost` everywhere:

- web UI: `http://localhost:3000`
- API: `http://localhost:8081`
- GitHub callback: `http://localhost:8081/api/ui/auth/github/callback`
- MCP endpoint: `http://localhost:8081/mcp`

Do not mix `localhost` and `127.0.0.1` during auth setup unless you deliberately reconfigure every related setting. The session and OAuth state cookies are host-specific, so a flow that starts on `localhost` and finishes on `127.0.0.1` will not behave reliably.

If you want to use `127.0.0.1` instead, update all of these together:

- `SPLAT_WEB_ORIGIN`
- `NEXT_PUBLIC_API_ORIGIN`
- `SPLAT_SERVER_CORS`
- the GitHub OAuth callback URL
- the browser URL you actually open
- the MCP URL you hand to clients

## 2. Create a GitHub OAuth App

Splat always boots the server with GitHub auth enabled, so local backend startup requires GitHub OAuth credentials.

Create a GitHub OAuth App with:

- Homepage URL: `http://localhost:3000`
- Authorization callback URL: `http://localhost:8081/api/ui/auth/github/callback`

Then copy the app's:

- client ID
- client secret

If you choose `127.0.0.1` instead of `localhost`, the callback URL must also use `127.0.0.1`.

## 3. Required Environment Variables

The repo loads `.env` via `mise`. A minimal local `.env` looks like this:

```dotenv
GITHUB_CLIENT_ID=your_github_oauth_app_client_id
GITHUB_CLIENT_SECRET=your_github_oauth_app_client_secret
SPLAT_WEB_ORIGIN=http://localhost:3000
NEXT_PUBLIC_API_ORIGIN=http://localhost:8081
SPLAT_SERVER_CORS=http://localhost:3000,http://127.0.0.1:3000
SPLAT_DATABASE_URL=postgres://splat:splat@localhost:5432/splat?sslmode=disable
SPLAT_TEST_DATABASE_URL=postgres://splat:splat@localhost:5432/splat_test?sslmode=disable
```

Notes:

- `GITHUB_CLIENT_ID` and `GITHUB_CLIENT_SECRET` are required for server startup.
- `SPLAT_WEB_ORIGIN` controls where the backend redirects the browser after GitHub login succeeds.
- `NEXT_PUBLIC_API_ORIGIN` tells the frontend where to send API requests. If this points at `localhost`, open the web app on `localhost` too.
- `SPLAT_SERVER_CORS` must include the exact browser origin that will call the API.
- `SPLAT_DATABASE_URL` is required for the Go backend.

## 4. Start the App Locally

Install toolchains and JS dependencies:

```bash
mise install
npm install
```

Start the full local stack:

```bash
mise run dev
```

That launches:

- PostgreSQL via `docker compose`
- the Go API on `http://localhost:8081`
- the Next.js app on `http://localhost:3000`

Then open `http://localhost:3000` and sign in with GitHub.

If you only run `npm run dev`, you will get the frontend only. OAuth, API, token management, and MCP all depend on the Go server running.

## 5. Sign In and Mint a Personal API Token

The browser uses the GitHub-backed session cookie. MCP clients do not.

For MCP, create a personal API token after signing in:

1. Open `http://localhost:3000/settings`
2. Click `Create token`
3. Copy the token immediately

The full token is only shown once. Stored tokens are listed later by prefix only.

Splat's MCP endpoint is:

```text
http://localhost:8081/mcp
```

It is bearer-token only. Send:

```text
Authorization: Bearer spt_...
```

Do not expect the browser session cookie to authenticate `/mcp`.

## 6. Configure Codex

Recommended: store the token in an environment variable and point Codex at it.

```bash
export SPLAT_MCP_TOKEN='replace-with-your-token'
codex mcp remove splat
codex mcp add splat --url http://localhost:8081/mcp --bearer-token-env-var SPLAT_MCP_TOKEN
codex mcp get splat
```

The equivalent `~/.codex/config.toml` entry looks like:

```toml
[mcp_servers.splat]
url = "http://localhost:8081/mcp"
bearer_token_env_var = "SPLAT_MCP_TOKEN"
```

## 7. Configure Claude Code

Claude Code can talk to the same authenticated HTTP MCP endpoint by storing an `Authorization` header in its MCP config:

```bash
claude mcp remove -s local splat
claude mcp add --scope local --transport http splat http://localhost:8081/mcp \
  --header "Authorization: Bearer $SPLAT_MCP_TOKEN"
claude mcp get splat
```

Use `--scope user` instead of `--scope local` if you want the MCP server available across repositories instead of just on this machine's local config for the current project context.

Important:

- the shell expands `$SPLAT_MCP_TOKEN` before Claude stores the config
- rotating the token means rerunning `claude mcp add ... --header ...`
- if you use `localhost` for auth and web access, use `localhost` here too

## 8. Common Failure Modes

`server fails to start with "github client id is required"` or `"github client secret is required"`

- `GITHUB_CLIENT_ID` and `GITHUB_CLIENT_SECRET` are missing or blank.

GitHub login redirects, then you end up unauthenticated again

- You mixed `localhost` and `127.0.0.1`.
- `SPLAT_WEB_ORIGIN` does not match the hostname you are browsing.
- The GitHub OAuth callback URL does not match the backend hostname.

The web app loads, but API calls return `401` or never pick up your session

- `NEXT_PUBLIC_API_ORIGIN` points at a different host than the one your browser used for login.
- `SPLAT_SERVER_CORS` is missing your frontend origin.

The MCP client connects but every tool call returns `authentication required`

- The client is missing `Authorization: Bearer ...`.
- You revoked the token and need to mint a new one.
- You pointed the client at the wrong host or port.
