<!-- stackit:start -->
## Git Workflow: Stacked PRs

This project uses [stackit](https://github.com/getstackit/stackit) for stacked changes.
AI agents should proactively work in stacks.

### Why Stack?
Small PRs get reviewed faster. Break features into focused, reviewable units.

### When to Stack
Stack when your change has 2+ logical phases, exceeds ~400 lines,
or would benefit from early review of foundational work.

### Workflow
```bash
git add -A                        # Stage first
stackit create -m "feat: ..."     # Create stacked branch
# ... continue working ...
stackit submit                    # Submit all PRs
```

### Key Commands
| Command | Purpose |
|---------|---------|
| `stackit create -m "msg"` | Create stacked branch |
| `stackit submit` | Push & create/update PRs |
| `stackit sync` | Pull trunk, cleanup merged |
| `stackit log` | Visualize branch tree |

Run `/stackit` for the full skill, or `/stack-status` to check current state.
<!-- stackit:end -->

## Validation & Pre-Submit Gate

**Run `mise run check` and get it green before every `stackit submit`.** Submitting a
red PR wastes a review cycle and, in a stack, blocks every PR above it. CI runs
golangci-lint, eslint, schema/sqlc drift checks, backend tests (`go test`), web
tests (vitest), and the web build — `mise run check` mirrors that locally.

```bash
mise run check        # fmt + lint + schema/sqlc drift + backend tests + web tests + build
stackit submit        # only when check is green
```

- Backend tests and the schema-drift check need PostgreSQL up (`docker compose up -d`).
- After editing a DB migration, regenerate generated artifacts (`mise run
  generate:schema`, `mise run generate:sqlc`) and commit them, or the drift checks
  fail CI's `Test` job before tests even run.
- In a stack, fix a failure in the branch that introduced it (`stackit absorb`, or
  `stackit checkout <branch>` → edit → `stackit modify -a`) so it propagates up via
  restack — a green tip with red ancestors still fails the ancestor PRs' CI.

See `.claude/rules/validation.md` (validation levels, golangci cache caveat) and
`.claude/rules/stackit-workflow.md` (stack commands, fix routing) for detail.
