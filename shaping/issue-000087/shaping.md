---
shaping: true
issue: issue-000087
---

# Multi-Project / Multi-Repo Scope — Shaping

## Requirements (R)

| ID | Requirement | Status |
|----|-------------|--------|
| R0 | Issues can be grouped by scope (repo, project, product area, team, or none) | Core goal |
| R1 | An issue can belong to multiple scopes simultaneously (e.g., a team + a product area) | Must-have |
| R2 | Users can filter issues by scope, and scopes are composable (team AND repo) | Must-have |
| R3 | Scopes can optionally be associated with a git repository, but many scopes won't be | Must-have |
| R4 | Unscoped browsing is the default — scope is a lens, not a container or wall | Must-have |
| R5 | Scope is optional on issues — zero scopes is normal | Must-have |
| R6 | Scope can be inferred from context (e.g., current repo via .sortit.yaml) | Leaning yes |

---

## Shapes

### A: Scopes are just tags

No new concept. Use tag conventions (e.g., `repo:sortit`, `team:platform`) and filtering syntax.

| Part | Mechanism |
|------|-----------|
| **A1** | Convention: scope tags use a prefix pattern (`repo:X`, `team:Y`, `area:Z`) |
| **A2** | CLI `--scope` flag translates to tag filter |
| **A3** | CLI detects current git repo and suggests/applies `repo:` tag on creation |
| **A4** | Enrichment can infer scope tags the same way it infers any tag |

### B: First-class scope entity

New `scopes` table with many-to-many join to issues. Tags remain separate.

| Part | Mechanism |
|------|-----------|
| **B1** | `scopes` table: id, name, type (repo/team/area/custom), repo_url (nullable) |
| **B2** | `issue_scopes` join table: issue_id, scope_id |
| **B3** | CLI `--scope` flag filters by scope entity |
| **B4** | CLI detects current repo → matches against known scopes with repo_url |
| **B5** | Scope management commands (`sortit scopes create/list/delete`) |

### C: Typed tags ← Selected

Extend the existing tag system with a `dimension` field. Scope-dimension tags get special filtering and boosting behavior but share the same infrastructure.

| Part | Mechanism | Flag |
|------|-----------|:----:|
| **C1** | Add `dimension` column to `tags` table: `scope` or `content` (default). Scope tags created explicitly via `sortit tags create --dimension scope`. | |
| **C2** | Scope tags always get relevance 1.0 on issues — binary, not continuous. | |
| **C3** | `--scope` flag on list/search filters by scope-dimension tags only, with AND semantics. | |
| **C4** | `.sortit.yaml` in repo root (searched upward). Default scopes auto-applied on issue creation. For search/explore, default scopes act as a **boost** (not filter) — issues matching your scope rank higher, but out-of-scope issues still appear. Explore surfaces when related issues have incompatible scopes. | |
| **C5** | Enrichment can suggest scope tags the same as content tags. Scope tags get 1.0 loading when applied. Nuance to be resolved during build. | |

---

## Fit Check: R × C (Selected Shape)

| Req | Requirement | Status | C |
|-----|-------------|--------|---|
| R0 | Issues can be grouped by scope (repo, project, product area, team, or none) | Core goal | ✅ |
| R1 | An issue can belong to multiple scopes simultaneously | Must-have | ✅ |
| R2 | Users can filter issues by scope, and scopes are composable | Must-have | ✅ |
| R3 | Scopes can optionally be associated with a git repository | Must-have | ✅ |
| R4 | Unscoped browsing is the default — scope is a lens, not a container | Must-have | ✅ |
| R5 | Scope is optional on issues — zero scopes is normal | Must-have | ✅ |
| R6 | Scope can be inferred from context (e.g., current repo via .sortit.yaml) | Leaning yes | ✅ |

---

## Rejected Shapes

- **A** fails R2 (can't distinguish scope tags from content tags in filters), R3 (no metadata for repo association), R6 (no registry mapping repos to tags).
- **B** works but adds a separate entity and management surface when tags already provide the right multi-membership model.
