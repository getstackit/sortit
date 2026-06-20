# Validation Strategy

Use the lightest validation that covers your change while iterating. But **before a PR
is created or updated** (`stackit submit`), the full gate is mandatory — see
[Pre-Submit Gate](#pre-submit-gate-required-before-stackit-submit) below.

All commands are `mise` tasks defined in `mise.toml`.

## Validation Levels (Fastest to Slowest)

| Level | Command | Covers | Use When |
|-------|---------|--------|----------|
| Compile | `mise run compile` | `go build ./apps/... ./cmd/... ./internal/...` | Docs, comments, type/import changes |
| Go lint | `mise run lint:go` | golangci-lint | Go refactors, style changes |
| Web lint | `mise run lint:web` | eslint | Web refactors, style changes |
| Lint (both) | `mise run lint` | golangci-lint + eslint | Any lint-only verification |
| Backend tests | `mise run test:fast` | `go test ./apps/... ./cmd/... ./internal/...` | Go logic changes (needs test DB) |
| Web checks | `mise run check:web` | `npm run test` + `npm run build` | Web component / API-contract changes |
| Go checks | `mise run check:go` | fmt + lint:go + compile + test:fast | Multi-package Go changes |
| **Full check** | `mise run check` | fmt + lint + test:fast + check:web | **Before every `stackit submit`** |

> The backend tests (`test:fast`, `check:go`, `check`) need PostgreSQL up
> (`docker compose up -d`), matching `SORTIT_TEST_DATABASE_URL`. If the DB is
> down, start it first — a skipped/failed DB is not a passing run.

## Decision Guide

- **Comment/doc/type-only change** → `mise run compile`
- **Renamed/extracted/reorganized, no behavior change** → `mise run lint`
- **Logic change in Go packages** → `mise run check:go`
- **React component / hook / lib change** → `mise run check:web`
- **Web + Go API contract change** → `mise run check`
- **About to `stackit submit`** → `mise run check` (always)

## Pre-Submit Gate (REQUIRED before `stackit submit`)

CI runs golangci-lint, eslint, backend tests, web tests, and the web build. A red
PR wastes a review cycle and blocks the rest of the stack. **Never run `stackit
submit` until `mise run check` passes locally.**

```bash
mise run check        # fmt + lint + backend tests + web tests + build
# only when green:
stackit submit
```

When working a **stack**, the lint/test that matters is each branch's full tree, not
just the tip. If `mise run check` on the tip surfaces an issue introduced in an
earlier branch, fix it in that branch (`stackit absorb`, or `stackit checkout
<branch>` + edit + `stackit modify -a`) so the fix propagates up via restack — then
re-run `mise run check` on the tip before submitting. A green tip with red ancestors
still fails CI on the ancestor PRs.

### Notes on golangci-lint caching

golangci-lint's incremental cache can produce **false** cross-file `goconst` counts
when only some files in a package were re-analyzed. If a `goconst` finding looks
spurious (a literal in an unrelated, unchanged test file), re-run with a cold cache
before acting:

```bash
rm -rf /tmp/sortit-golangci-lint-cache
mise run lint:go
```

A clean full run is the source of truth — it matches CI.

## Escalation

If a lighter validation passes but you're uncertain what the change touched,
escalate to the next level. **Rule of thumb:** match validation scope to change
scope — but the pre-submit gate is non-negotiable.
