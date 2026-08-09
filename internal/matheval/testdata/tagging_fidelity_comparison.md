# WP-702 tagging-fidelity comparison

Generated: 2026-08-08  \nFixture: matheval real corpus (48 issues, 16 tags; committed text-embedding-3-small vectors)  \nCommand: `go run ./internal/matheval/cmd/tageval -live -models gpt-5.4-nano -runs 1 -date 2026-08-08 -out-dir internal/matheval/testdata`

Fidelity uses `AnalysisTrace.ModelOutput` after the production relevance floor (0.08), before verifier/post-processing. Ranking substitutes the verifier's final tag scores into the full uncentered ridge path. Ranges are min–max across three serial model runs.

| Model | Micro F1 | Macro F1 | Correct / incorrect relevance | Negation FP | NDCG@8 Δ | Recall@8 Δ | Tokens in / out per issue | $ / 1k enrichments |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| gpt-5.4-nano | 0.739 [0.739–0.739] | 0.696 [0.696–0.696] | 0.710 / 0.466 | 0/0 (0.0%) | -0.126 | -0.203 | 1205.4 / 107.3 | $0.375 |

The JSON table for each model includes all 48 per-issue precision/recall/F1 rows, pre-verifier assignments and negations, final verified scores, and per-issue provider token usage.
