# Sortit Documentation

Sortit is an issue tracker for freeform input. Users paste a bug report, feature idea, stack trace, customer quote, or working note; Sortit turns it into a searchable, tagged, related issue record.

These docs describe the current product and implementation. Historical planning notes and shaping documents are intentionally not preserved here.

## Guides

- [Architecture](./architecture.md): main runtime components and request flow.
- [Development](./development.md): local setup, environment variables, auth, and common commands.
- [API, CLI, and MCP](./api-cli-mcp.md): supported integration surfaces.
- [Scoring, Search, and Map](./scoring-search-map.md): how tags, embeddings, ranking, and projection work.
- [Data Model](./data-model.md): PostgreSQL tables, append-only facts, projections, and migrations.
- [Planning](./planning.md): forward-looking design sketch for the quantitative project management layer.

## Quick Start

```bash
mise install
npm install
mise run dev
```

The full development stack starts PostgreSQL, the Go API on `http://localhost:8081`, and the Next.js app on `http://localhost:3000`.

GitHub OAuth credentials are required for local backend startup. See [Development](./development.md) for the complete setup.
