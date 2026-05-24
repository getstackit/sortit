# Contributing to Sortit

Thank you for your interest in contributing to Sortit! This document outlines the process for contributing to the project.

## Getting Started

### Prerequisites

- **Git 2.25+**
- **GitHub CLI (`gh`)** for PR operations
- **Docker** (for the local PostgreSQL container)
- **[mise](https://mise.jdx.dev/)** - Tool version manager and task runner

### Quick Setup

1. Install mise:
   ```bash
   # macOS/Linux
   curl https://mise.run | sh

   # Or via Homebrew
   brew install mise

   # Add to your shell (bash/zsh)
   echo 'eval "$(mise activate bash)"' >> ~/.bashrc   # or ~/.zshrc for zsh
   ```

2. Install all project tools and dependencies:
   ```bash
   mise install
   npm install
   ```

This installs Go, Node, goimports, golangci-lint, overmind, and watchexec.

### Setting Up Your Development Environment

1. Fork and clone the repository:
   ```bash
   git clone https://github.com/your-username/sortit.git
   cd sortit
   ```

2. Install tools and dependencies:
   ```bash
   mise install
   npm install
   ```

3. Configure environment variables and a GitHub OAuth app — see the [Development section of the README](README.md#development) for the full local setup, including `.env` contents and OAuth callback URLs.

4. Build the project:
   ```bash
   mise run build
   ```

5. Run the app:
   ```bash
   mise run dev
   ```

   This starts PostgreSQL via `docker compose`, the Go API on `http://localhost:8081`, and the Next.js app on `http://localhost:3000`.

## Development Workflow

1. Create a topic branch from `main`:
   ```bash
   git checkout -b your-feature-name
   ```

2. Make your changes and commit them following the [Commit Message Format](#commit-message-format) below.

3. Push your branch and open a Pull Request against `main`.

### Running Tests and Linting

Before submitting your changes, ensure all checks pass:

```bash
# Run everything (format, lint, backend tests, frontend tests, build)
mise run check

# Or run individually:
mise run fmt          # Format Go code
mise run lint         # Run ESLint and golangci-lint
mise run test         # Backend + frontend tests
mise run test:fast    # Backend tests only
mise run check:web    # Frontend tests + Next.js build
mise run check:go     # Go format, lint, compile, backend tests

# View all available tasks:
mise tasks
```

All changes must pass `mise run check` before being submitted.

### Database schema

The Go backend uses sqlc with a checked-in schema snapshot at `internal/issues/sqlc/schema.sql`. If you change migrations, regenerate the snapshot and verify it matches:

```bash
mise run generate:schema
mise run check:schema-drift
```

## Commit Message Format

Sortit uses [Conventional Commits](https://www.conventionalcommits.org/) for all commit messages.

### Format

```
<type>[optional scope]: <description>
```

### Types

- `feat`: A new feature
- `fix`: A bug fix
- `docs`: Documentation only changes
- `style`: Code style changes (formatting, missing semi-colons, etc.)
- `refactor`: Code refactoring without changing functionality
- `perf`: Performance improvements
- `test`: Adding or updating tests
- `chore`: Maintenance tasks, dependency updates, etc.
- `ci`: Changes to CI configuration

### Examples

```
feat: add tag covariance to map layout
fix: resolve drift in schema export
refactor: simplify issue enrichment pipeline
docs: update README with MCP setup
test: add tests for issue search
chore: update dependencies
```

### Best Practices

- Use the imperative mood ("add" not "added" or "adds")
- Keep the description concise but descriptive
- Reference issues or PRs when applicable: `feat: add feature (#123)`
- Use the scope when it helps clarify the change: `feat(map): add edge filtering`

## Submitting Changes

1. Ensure your code passes all checks:
   ```bash
   mise run check
   ```

2. Push your branch and open a Pull Request on GitHub with a clear description of your changes.

3. Ensure your PR description follows the same conventions as commit messages when possible.

## Code Style

### Go

- Follow Go standard formatting (use `mise run fmt` or `goimports`)
- Prefer early returns over deep nesting
- Always handle errors explicitly; never ignore them with `_`
- Wrap errors with context: `fmt.Errorf("context: %w", err)`

### TypeScript / React

- Run `mise run lint:web` (ESLint) before committing
- Follow the existing component structure under `src/`

## Project Structure

```
sortit/
├── apps/
│   ├── cli/        # Sortit CLI binary
│   └── server/     # Go API and MCP server
├── cmd/            # Schema tooling (drift check, schema export, migrations)
├── internal/
│   ├── ai/             # AI provider integrations (tagging, canonicalization, embeddings)
│   ├── api/            # HTTP handlers and routing
│   ├── auth/           # GitHub OAuth and personal API tokens
│   ├── cli/            # CLI command implementations
│   ├── domain/         # Core domain types
│   ├── issues/         # Issue storage (sqlc + PostgreSQL)
│   ├── issueenrichment/ # Tag scoring and embedding pipeline
│   ├── issuemath/      # Factor model and similarity math
│   ├── map/            # 2D layout (PCA) and map generation
│   ├── mcp/            # MCP server for Claude / Codex integration
│   └── ...             # See the directory for the full list
├── src/            # Next.js frontend (React, Tailwind, shadcn/ui)
└── docker/         # Local PostgreSQL container config
```

## Philosophy

When contributing, keep these principles in mind:

1. **Just paste it in**: The product surface should not require forms, fields, or manual categorization. New features should preserve the "dump text in, system figures it out" feel.
2. **Stable layout**: The map's positioning model should change predictably. Avoid changes that cause large unrelated issues to jump around the map without a clear reason.
3. **AI is a component, not the product**: Tagging, embeddings, and canonicalization are inputs to a deterministic factor model. Keep the math layer testable and independent of the AI provider.
4. **Backend-first persistence**: All durable state lives in PostgreSQL behind the Go API. The frontend should not hold state that the backend cannot reconstruct.

## Questions?

If you have questions about contributing, please open an issue on GitHub or reach out to the maintainers.

Thank you for contributing to Sortit!
