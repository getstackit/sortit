# Architecture

Sortit has three runtime surfaces over the same issue domain:

- a Next.js web app in `src/`
- a Go HTTP API in `apps/server` and `internal/api`
- a Go CLI in `apps/cli`, plus an authenticated MCP endpoint at `/mcp`

The backend owns persistence, authentication, enrichment, search, map data, and issue mutations. The frontend is a client of `/api/ui`. External callers use the dedicated API namespace, the CLI, or MCP.

## Main Flow

1. A user or agent submits freeform issue text.
2. The backend embeds the text and builds a retrieval-first tag candidate set from the stored tag catalog.
3. The AI analyzer scores candidate tags and returns an embedding.
4. The enrichment layer decorates tag scores with candidate provenance, specificity, alignment, and source-text evidence.
5. Issue mutations persist the canonical issue state, discussion posts, operations, links, append-only facts, projections, and activity events in PostgreSQL.
6. Search, map, people, and detail surfaces read from those persisted records.

## Backend Packages

- `internal/api`: HTTP server, routes, auth middleware, request/response handlers.
- `internal/issues`: issue domain types, stores, migrations, lifecycle/content/tag persistence.
- `internal/issues/commands`: transactional mutation handlers for create, refine, progress, close, assign, split, combine, link, and re-enrich.
- `internal/issues/views`: read models for issue detail, lists, activity, tags, lifecycle metrics, and comparisons.
- `internal/issueenrichment`: AI-backed canonicalization, tagging, verification, and background enrichment jobs.
- `internal/ai`: analyzer abstraction, OpenAI implementation, and test stubs.
- `internal/tags`: tag catalog, retrieval shortlists, embeddings, specificity scoring, and tag merge support.
- `internal/map`, `internal/mapview`, `internal/issuemath`: factor decomposition, map projection, search/explore similarity, and map API data.
- `internal/issueanalytics`: deterministic scoring primitives such as content confidence, maturity, velocity, freshness, authority, and hubness.
- `internal/search`: dedicated and unified search handlers.
- `internal/people`: person profiles, detail views, recommendations, and work correlations.
- `internal/auth`: GitHub OAuth sessions, API tokens, CLI login, and bearer-token auth.
- `internal/mcp`: Streamable HTTP MCP tools backed by the same command and query handlers.

## Frontend Structure

- `src/app`: Next.js routes for issues, map, tags, people, analytics, activity, settings, auth, and debug pages.
- `src/components`: shared application shell and feature components.
- `src/lib`: typed API clients, formatting helpers, auth helpers, search utilities, and tag-quality helpers.
- `src/features/map`: frontend map model, API adapter, URL state, and tests.

The UI uses React, Next.js, Tailwind, shadcn/ui-style primitives, SWR, and lucide icons.

## API Namespaces

The server registers two authenticated API namespaces:

- `/api/ui`: browser-oriented API used by the Next.js app. Includes UI-only routes such as revision streaming, map data, activity feed, debug analysis, and unified search.
- `/api/v1` and `/api`: dedicated API namespaces for CLI and external clients.

Both namespaces share the same auth service and domain handlers. Public routes such as health and GitHub OAuth start/callback are available before auth middleware. Mutating issue, tag, people, debug, token, and search routes require authentication.

## Enrichment

Issue enrichment is retrieval-first:

- The issue text is embedded before tag classification.
- The catalog service selects nearest tag embeddings plus a stable anchor set and any explicit tags.
- The analyzer scores only that candidate set by default.
- Few-shot examples can be selected from the exemplar pool when available.
- Verification uses tag alignment, specificity, candidate source, and grounded evidence to keep, down-rank, or flag tags.

The server can also run a background enrichment worker for queued enrichment jobs. Mutations invalidate map projections and bump a revision counter so clients can refresh.

## Persistence Boundary

PostgreSQL is the application database. The current schema keeps compatibility tables such as `issues` and `tags` while also introducing append-only fact/projection tables for lifecycle, content, enrichment, and tag history. Reads can stay fast against projections while durable history remains replayable.
