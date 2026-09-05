# WP-702 tagging-fidelity comparison

Generated: 2026-08-11

Fixture: matheval real corpus (48 issues, 16 tags; committed text-embedding-3-small vectors)

Command: `go run ./internal/matheval/cmd/tageval -live -models gpt-5.6,gpt-5.6-luna,gpt-5.6-terra -runs 3 -date 2026-08-11 -out-dir internal/matheval/testdata`

Fidelity uses `AnalysisTrace.ModelOutput` after the production relevance floor (0.08), before verifier/post-processing. Ranking substitutes the verifier's final tag scores into the full uncentered ridge path. The ranking baseline uses the fixture's ground-truth tag scores, so live models show a negative Δ; use the between-model comparison rather than treating that sign as a disqualification. Ranges are min–max across 3 serial model runs.

| Model | Micro F1 | Macro F1 | Correct / incorrect relevance | Negation FP | NDCG@8 Δ | Recall@8 Δ | Tokens in / out per issue | $ / 1k enrichments |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| gpt-5.6 | 0.743 [0.734–0.758] | 0.696 [0.687–0.711] | 0.912 / 0.869 | 0/0 (0.0%) | -0.076 | -0.081 | 1205.4 / 356.0 | $16.708 |
| gpt-5.6-luna | 0.788 [0.783–0.792] | 0.743 [0.741–0.744] | 0.933 / 0.890 | 0/0 (0.0%) | -0.056 | -0.074 | 1205.4 / 310.7 | $3.070 |
| gpt-5.6-terra | 0.854 [0.842–0.874] | 0.804 [0.792–0.825] | 0.885 / 0.707 | 0/0 (0.0%) | -0.036 | -0.049 | 1205.4 / 103.7 | $4.569 |

The JSON table for each model includes all 48 per-issue precision/recall/F1 rows, pre-verifier assignments and negations, final verified scores, and per-issue provider token usage.
