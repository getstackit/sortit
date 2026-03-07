# bored

An issue tracker where you just dump text in and the system figures out the rest.

## How it works

You paste anything — a bug report, a feature idea, a stack trace, a customer quote — into a single text box. The system automatically:

1. **Extracts tags** with continuous relevance scores (e.g. an issue might be 0.8 "bug", 0.3 "ui", 0.1 "performance")
2. **Generates embeddings** from the raw text
3. **Places the issue on a 2D map** that clusters similar issues together

No forms, no fields, no manual categorization.

## The Map

The map visualizes all issues on a 2D surface. Position is determined automatically using a **factor model** inspired by quantitative finance.

### Relevance model

Each issue is decomposed into tag relevance scores — continuous values (0 to 1) representing how strongly an issue relates to each tag. These are AI-inferred from the issue text.

```
issue_i = sum(relevance_ij * tag_j) + residual_i
```

Where:
- **Tags** are the dimensions (bug, ui, performance, feature, ...)
- **Relevance scores** are continuous values — an issue can be 0.8 "bug" and 0.3 "ui" simultaneously
- **Tag covariance** captures how tags relate to each other (see below)
- **Residual** is what makes an issue unique beyond its tags

### Tag covariance

Tags are not independent dimensions — "bug" and "crash" are semantically closer than "bug" and "onboarding." The tag covariance matrix Σ_tags (T×T) captures these relationships.

**Source:** Each tag is embedded using the same embedding model used for issues (e.g., OpenAI embeddings). The covariance between two tags is the cosine similarity of their embeddings. A tag can be embedded from its name alone, or from a short description (e.g., "bug - software defect") for disambiguation.

**Why embeddings, not co-occurrence:** Deriving correlations from how often tags appear together in the data would make the matrix shift as issues are added, and would be unreliable when the dataset is small. Embedding-derived correlations are stable — they reflect the semantic relationship between tags regardless of what issues exist. They also work immediately for newly created tags.

**Effect on positioning:** Before PCA, the issue-tag relevance matrix X (N×T) is transformed by the tag covariance: X' = X × Σ_tags. This "smears" each issue's loadings across correlated tags. An issue tagged only with "crash" picks up implicit weight on "bug," pulling it closer to bug-related issues on the map. Without this step, tags are treated as orthogonal and the map misses known semantic relationships.

**Lifecycle:** The covariance matrix is recomputed when the tag set changes (a tag is added, removed, or renamed). It does not need to update when issues change. This makes it cheap — one embedding call per tag.

### Similarity

Issue similarity is computed by blending two sources:

1. **Relevance-based similarity**: `Sigma_issues = R * Sigma_tags * R' + D`
   - Captures structural similarity through shared tag relevance
   - Interpretable — you can explain *why* two issues are close
2. **Embedding similarity**: cosine distance between text embeddings
   - Captures semantic similarity in the raw text
   - Catches relationships the tag structure might miss

### Layout: PCA

The blended similarity matrix is projected to 2D using **PCA** (principal component analysis).

We chose PCA over alternatives (UMAP, t-SNE, MDS) for one reason: **stability**. Adding or removing an issue shouldn't reshuffle the entire map. PCA is deterministic and changes incrementally — existing issues stay roughly where they were. This matters for a tool people use daily; spatial memory is valuable.

The tradeoff is that PCA is linear, so it can collapse clusters that are distinct in high dimensions. In practice, the factor model's tag structure provides enough separation that this isn't a major issue.

### Edges

Edges between issues represent **embedding similarity** — semantic closeness in the raw text, independent of tags.

This creates two complementary layers:
- **Position** = factor model (tag relevance + covariance). Structural, interpretable.
- **Edges** = embedding similarity. Semantic, content-based.

Two issues can be far apart on the map (different tags) but connected by an edge because the text is semantically similar. This surfaces relationships the factor model misses. Conversely, issues close together but with no edge are structurally similar but actually about different things.

### Why not just embeddings?

Pure embedding similarity is a black box. The relevance model gives you interpretable structure — you can say "these issues cluster together because they're both highly relevant to ui and performance" rather than "the embedding said so." The embeddings fill in the gaps for relationships the tag structure doesn't capture.

## Stack

- **Frontend**: Next.js, React, Tailwind, shadcn/ui
- **Backend**: Go

## Development

```bash
npm run dev
```

Open [http://localhost:3000](http://localhost:3000).
