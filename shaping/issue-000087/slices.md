---
shaping: true
issue: issue-000087
---

# Multi-Project / Multi-Repo Scope — Slices

## Slice Dependency

```
V1 (scope tags + filter)
├── V2 (.splat.yaml + auto-apply)
└── V3 (boost + explore awareness)
```

---

## V1: Scope tags exist and filter

**Parts:** C1, C2, C3 (partial)

| Affordance | Detail |
|------------|--------|
| DB migration | Add `dimension` column to `tags` table (default: `content`) |
| CLI: create scope tag | `splat tags create --dimension scope "billing-service"` |
| CLI: list tags shows dimension | Scope tags visually distinguished in output |
| CLI: `--scope` flag on `issues list` | Filters to issues with ALL specified scope tags (AND), only matches scope-dimension tags |
| API | Tag creation accepts dimension, list filter accepts scope param |
| Scope tags get 1.0 relevance | When a scope tag is applied to an issue, relevance is always 1.0 |

**Demo:** Create scope tags `splat` and `team-platform`. Tag some issues. `splat issues list --scope splat` shows only those issues. `splat issues list --scope splat --scope team-platform` narrows further.

---

## V2: `.splat.yaml` and auto-apply on create

**Parts:** C4 (creation side)

| Affordance | Detail |
|------------|--------|
| `.splat.yaml` file format | `scopes: [billing-service, team-platform]` |
| CLI walks up directory tree | Finds nearest `.splat.yaml`, merges if multiple found |
| Auto-apply on issue create | Default scopes from `.splat.yaml` automatically applied to new issues |
| CLI shows active scopes | `splat status` or similar shows which `.splat.yaml` scopes are active |

**Demo:** Drop `.splat.yaml` with `scopes: [splat]` in repo root. `splat issues create "new bug"` → issue created with scope tag `splat` at 1.0 relevance automatically.

---

## V3: Scope boost on search/explore

**Parts:** C4 (query side), C5

| Affordance | Detail |
|------------|--------|
| Search boost | `.splat.yaml` scopes boost matching issues in search results (not filter) |
| Explore scope awareness | When exploring related issues, surface when a related issue has incompatible scopes (different or none) |
| Enrichment | Enrichment can suggest scope tags — applied at 1.0 when accepted |

**Demo:** From a repo with `.splat.yaml` scopes, `splat issues search "auth bug"` ranks in-scope issues higher. `splat issues explore issue-000087` flags that a related issue belongs to a different scope.
