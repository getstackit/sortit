# sortit

An issue tracker where you just dump text in and the system figures out the rest.

## How it works

You paste anything — a bug report, a feature idea, a stack trace, a customer quote — into a single text box. The system automatically:

1. **Extracts tags** with continuous relevance scores (e.g. an issue might be 0.8 "bug", 0.3 "ui", 0.1 "performance")
2. **Generates embeddings** from the raw text
3. **Places the issue on a 2D map** that clusters similar issues together

No forms, no fields, no manual categorization.

## Documentation

The public docs live in [`docs/`](./docs/):

- [Architecture](./docs/architecture.md)
- [Development](./docs/development.md)
- [API, CLI, and MCP](./docs/api-cli-mcp.md)
- [Scoring, Search, and Map](./docs/scoring-search-map.md)
- [Data Model](./docs/data-model.md)

## The Map

The map visualizes all issues on a 2D surface. Position is determined automatically using a **factor model** inspired by quantitative finance.

### Relevance model

Each issue is decomposed into tag relevance scores — continuous values (0 to 1) representing how strongly an issue relates to each tag. These are AI-inferred from the issue text.

```
issue_i = sum(relevance_ij * tag_j) + residual_i
```

Where:
- **Tags** are the dimensions (bug, ui, performance, feature, ...)
- **Relevance scores** are continuous values — an issue can be 0.8 "bug" and 0.3 "ui" simultaneously
- **Tag covariance** captures how tags relate to each other (see below)
- **Residual** is what makes an issue unique beyond its tags

### Tag covariance

Tags are not independent dimensions — "bug" and "crash" are semantically closer than "bug" and "onboarding." The tag covariance matrix Σ_tags (T×T) captures these relationships.

**Source:** Each tag is embedded using the same embedding model used for issues (e.g., OpenAI embeddings). The covariance between two tags is the cosine similarity of their embeddings. A tag can be embedded from its name alone, or from a short description (e.g., "bug - software defect") for disambiguation.

**Why embeddings, not co-occurrence:** Deriving correlations from how often tags appear together in the data would make the matrix shift as issues are added, and would be unreliable when the dataset is small. Embedding-derived correlations are stable — they reflect the semantic relationship between tags regardless of what issues exist. They also work immediately for newly created tags.

**Effect on positioning:** Before PCA, the issue-tag relevance matrix X (N×T) is transformed by the tag covariance: X' = X × Σ_tags. This "smears" each issue's loadings across correlated tags. An issue tagged only with "crash" picks up implicit weight on "bug," pulling it closer to bug-related issues on the map. Without this step, tags are treated as orthogonal and the map misses known semantic relationships.

**Lifecycle:** The covariance matrix is recomputed when the tag set changes (a tag is added, removed, or renamed). It does not need to update when issues change. This makes it cheap — one embedding call per tag.

### Similarity

Issue similarity is computed by blending two sources:

1. **Relevance-based similarity**: `Sigma_issues = R * Sigma_tags * R' + D`
   - Captures structural similarity through shared tag relevance
   - Interpretable — you can explain *why* two issues are close
2. **Embedding similarity**: cosine distance between text embeddings
   - Captures semantic similarity in the raw text
   - Catches relationships the tag structure might miss

### Layout: PCA

The blended similarity matrix is projected to 2D using **PCA** (principal component analysis).

We chose PCA over alternatives (UMAP, t-SNE, MDS) for one reason: **stability**. Adding or removing an issue shouldn't reshuffle the entire map. PCA is deterministic and changes incrementally — existing issues stay roughly where they were. This matters for a tool people use daily; spatial memory is valuable.

Raw PCA alone doesn't deliver that stability: eigenvector signs are arbitrary, and when the top two eigenvalues are close, PC1 and PC2 can swap between recomputes — either flips or rotates the whole map. Two mechanisms make stability real:

1. **Procrustes alignment.** Each rebuild is aligned to the previous layout: the optimal 2D rotation/reflection (orthogonal Procrustes, via SVD of the cross-covariance of issues present in both layouts) is applied to the new coordinates before normalization, so issues that didn't change stay where users last saw them.
2. **Deterministic orientation fallback.** When there is no previous layout (or fewer than 3 shared issues), each principal axis is oriented so the tag with the largest absolute loading points positive — a property of the data, not of input order.

The tradeoff is that PCA is linear, so it can collapse clusters that are distinct in high dimensions. In practice, the factor model's tag structure provides enough separation that this isn't a major issue.

### Edges

Edges between issues represent **embedding similarity** — semantic closeness in the raw text, independent of tags.

This creates two complementary layers:
- **Position** = factor model (tag relevance + covariance). Structural, interpretable.
- **Edges** = embedding similarity. Semantic, content-based.

Two issues can be far apart on the map (different tags) but connected by an edge because the text is semantically similar. This surfaces relationships the factor model misses. Conversely, issues close together but with no edge are structurally similar but actually about different things.

### Why not just embeddings?

Pure embedding similarity is a black box. The relevance model gives you interpretable structure — you can say "these issues cluster together because they're both highly relevant to ui and performance" rather than "the embedding said so." The embeddings fill in the gaps for relationships the tag structure doesn't capture.

## Stack

- **Frontend**: Next.js, React, Tailwind, shadcn/ui
- **Backend**: Go

## Development

### Prerequisites

```bash
mise install
npm install
```

This repo uses npm's stable `hoisted` install strategy via `.npmrc`. Because hoisted frontend dependencies can contain incidental Go files, project Go tasks target first-party package roots explicitly (`./apps/... ./cmd/... ./internal/...`) instead of raw `./...`.

### Pick one hostname and stick to it

The easiest local setup is to use `localhost` everywhere:

- web UI: `http://localhost:3000`
- API: `http://localhost:8081`
- GitHub callback: `http://localhost:8081/api/v1/auth/github/callback`
- MCP endpoint: `http://localhost:8081/mcp`

Do not mix `localhost` and `127.0.0.1` during auth setup unless you deliberately reconfigure every related setting. The session and OAuth state cookies are host-specific, so a flow that starts on `localhost` and finishes on `127.0.0.1` will not behave reliably.

If you want to use `127.0.0.1` instead, update all of these together:

- `SORTIT_WEB_ORIGIN`
- `NEXT_PUBLIC_API_ORIGIN`
- `SORTIT_SERVER_CORS`
- the GitHub OAuth callback URL
- the browser URL you actually open
- the MCP URL you hand to clients

### Create a GitHub OAuth app

Sortit always boots the server with GitHub auth enabled, so local backend startup requires GitHub OAuth credentials.

Create a GitHub OAuth App with:

- Homepage URL: `http://localhost:3000`
- Authorization callback URL: `http://localhost:8081/api/v1/auth/github/callback`

Then copy the app's:

- client ID
- client secret

If you choose `127.0.0.1` instead of `localhost`, the callback URL must also use `127.0.0.1`.

### Environment variables

The repo loads `.env` via `mise`. A minimal local `.env` looks like this:

```dotenv
GITHUB_CLIENT_ID=your_github_oauth_app_client_id
GITHUB_CLIENT_SECRET=your_github_oauth_app_client_secret
SORTIT_WEB_ORIGIN=http://localhost:3000
NEXT_PUBLIC_API_ORIGIN=http://localhost:8081
SORTIT_SERVER_CORS=http://localhost:3000,http://127.0.0.1:3000
SORTIT_DATABASE_URL=postgres://sortit:sortit@localhost:5432/sortit?sslmode=disable
SORTIT_TEST_DATABASE_URL=postgres://sortit:sortit@localhost:5432/sortit_test?sslmode=disable
```

Notes:

- `GITHUB_CLIENT_ID` and `GITHUB_CLIENT_SECRET` are required for server startup.
- `SORTIT_WEB_ORIGIN` controls where the backend redirects the browser after GitHub login succeeds.
- `NEXT_PUBLIC_API_ORIGIN` tells the frontend where to send API requests. If this points at `localhost`, open the web app on `localhost` too.
- `SORTIT_SERVER_CORS` must include the exact browser origin that will call the API.
- `SORTIT_DATABASE_URL` is required for the Go backend.

Optional AI configuration:

```dotenv
AI_PROVIDER=openai
OPENAI_API_KEY=your_openai_api_key
OPENAI_TAG_MODEL=gpt-5.4-nano
OPENAI_CANONICAL_MODEL=gpt-4.1-mini
OPENAI_EMBED_MODEL=text-embedding-3-small
```

`OPENAI_TAG_MODEL` scores tags for create/search flows. `OPENAI_CANONICAL_MODEL` rewrites refine/combine discussions into a canonical issue description before re-tagging. If you explicitly set `OPENAI_TAG_MODEL` but leave `OPENAI_CANONICAL_MODEL` blank, the backend preserves the old behavior and reuses the tag model for canonicalization.

### Start the app

```bash
mise run dev
```

That launches:

- PostgreSQL via `docker compose`
- the Go API on `http://localhost:8081`
- the Next.js app on `http://localhost:3000`

The default local endpoints are:

- Web UI: `http://localhost:3000`
- API: `http://localhost:8081`
- UI API namespace: `http://localhost:8081/api/ui`
- Dedicated API namespace: `http://localhost:8081/api/v1`
- MCP: `http://localhost:8081/mcp`
- PostgreSQL: `postgres://sortit:sortit@localhost:5432/sortit?sslmode=disable`

Then open `http://localhost:3000` and sign in with GitHub.

If you only run `npm run dev`, you will get the frontend only. OAuth, API, token management, and MCP all depend on the Go server running.

The Go API persists all data in PostgreSQL. Set `SORTIT_DATABASE_URL` or pass `-database-url` to the server. Tests run in disposable Testcontainers-managed databases. Local Postgres can be backed up with `mise run db:backup`, which writes a custom-format dump into `backups/postgres/`.

### Sign in and mint a personal API token

The browser uses the GitHub-backed session cookie. MCP clients do not.

For MCP, create a personal API token after signing in:

1. Open `http://localhost:3000/settings`
2. Click `Create token`
3. Copy the token immediately

The full token is only shown once. Stored tokens are listed later by prefix only.

Sortit's MCP endpoint is:

```text
http://localhost:8081/mcp
```

It is bearer-token only. Send:

```text
Authorization: Bearer sortit_...
```

Do not expect the browser session cookie to authenticate `/mcp`.

### CLI

For the CLI, authenticate by running:

```bash
go run ./apps/cli auth login
```

That opens the Sortit UI, uses your Sortit session to mint a personal API token, and stores it in your local CLI config.

### Configure Codex

Recommended: store the token in an environment variable and point Codex at it.

```bash
export SORTIT_MCP_TOKEN='replace-with-your-token'
codex mcp remove sortit
codex mcp add sortit --url http://localhost:8081/mcp --bearer-token-env-var SORTIT_MCP_TOKEN
codex mcp get sortit
```

The equivalent `~/.codex/config.toml` entry looks like:

```toml
[mcp_servers.sortit]
url = "http://localhost:8081/mcp"
bearer_token_env_var = "SORTIT_MCP_TOKEN"
```

### Configure Claude Code

Claude Code can talk to the same authenticated HTTP MCP endpoint by storing an `Authorization` header in its MCP config:

```bash
claude mcp remove -s local sortit
claude mcp add --scope local --transport http sortit http://localhost:8081/mcp \
  --header "Authorization: Bearer $SORTIT_MCP_TOKEN"
claude mcp get sortit
```

Use `--scope user` instead of `--scope local` if you want the MCP server available across repositories instead of just on this machine's local config for the current project context.

Important:

- the shell expands `$SORTIT_MCP_TOKEN` before Claude stores the config
- rotating the token means rerunning `claude mcp add ... --header ...`
- if you use `localhost` for auth and web access, use `localhost` here too

### Troubleshooting

`server fails to start with "github client id is required"` or `"github client secret is required"`

- `GITHUB_CLIENT_ID` and `GITHUB_CLIENT_SECRET` are missing or blank.

GitHub login redirects, then you end up unauthenticated again

- You mixed `localhost` and `127.0.0.1`.
- `SORTIT_WEB_ORIGIN` does not match the hostname you are browsing.
- The GitHub OAuth callback URL does not match the backend hostname.

The web app loads, but API calls return `401` or never pick up your session

- `NEXT_PUBLIC_API_ORIGIN` points at a different host than the one your browser used for login.
- `SORTIT_SERVER_CORS` is missing your frontend origin.

The MCP client connects but every tool call returns `authentication required`

- The client is missing `Authorization: Bearer ...`.
- You revoked the token and need to mint a new one.
- You pointed the client at the wrong host or port.
