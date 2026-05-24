# Security Policy

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Report security issues via **GitHub private vulnerability reporting**: use the **Report a vulnerability** button under the [Security tab](https://github.com/getstackit/sortit/security) of this repository.

Please include:

- A description of the issue and the impact.
- Steps to reproduce, ideally a minimal proof of concept.
- The affected version or commit (`main` is generally the only supported branch; see below).
- Any relevant logs, payloads, or screenshots.

We aim to acknowledge new reports within **3 business days** and to provide an initial triage response within **7 business days**. Coordinated disclosure timelines are agreed on per-report.

## Supported Versions

Sortit is in active pre-1.0 development. Only the latest commit on `main` is supported with security fixes. Pinned images, tagged releases, and forks are not maintained.

| Version          | Supported          |
| ---------------- | ------------------ |
| `main` (latest)  | :white_check_mark: |
| Older commits    | :x:                |

## Vulnerability Checks in CI

The following automated checks run in CI on every push and pull request:

- `npm audit --omit=dev` for production JavaScript dependencies.
- `govulncheck` against the shipped Go binaries (`./apps/cli/...`, `./apps/server/...`, `./cmd/...`).

A failing check blocks the relevant job and is treated as a release blocker. Contributors can run the same checks locally:

```bash
npm audit --omit=dev
mise run vuln:go
```

## Hardening Notes

- Container images are pinned to specific tags (see `docker-compose.yml` and `.github/workflows/test.yml`); do not switch to floating tags such as `latest`.
- Local-only debug surfaces (`/debug` routes and the Debug nav entry) are not intended for production deployments.
- Personal API tokens and GitHub OAuth secrets must never be committed; the canonical template is `.env.example` (or the values documented in the README until that file lands).

## Acknowledgements

Security researchers who responsibly disclose issues will be credited in release notes once the fix ships, unless they request anonymity.
