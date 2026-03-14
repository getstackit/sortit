"use client";

import Link from "next/link";
import { Fragment, useEffect, useMemo, useState } from "react";
import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import { Skeleton } from "@/components/ui/skeleton";
import { useIssues } from "@/hooks/use-issues";
import {
  buildConsolidationCandidates,
  buildMergeCandidates,
  buildSpecificTagSuggestions,
  cosineSimilarity,
  isGenericBucketTag,
} from "@/lib/tag-quality";
import { Badge } from "@/components/ui/badge";
import { fetchTags, tagHref, type TagRecord } from "@/lib/tags";

type TagPoint = {
  tag: TagRecord;
  x: number;
  y: number;
};

type TagEdge = {
  source: string;
  target: string;
  similarity: number;
};

const SECTION_LINKS = [
  { id: "controls", title: "Controls" },
  { id: "projection", title: "Projection" },
  { id: "consolidation", title: "Consolidation" },
  { id: "matrix", title: "Matrix" },
];

export default function TagsPage() {
  const [tags, setTags] = useState<TagRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [edgeThreshold, setEdgeThreshold] = useState(0.72);
  const [selectedTagName, setSelectedTagName] = useState<string | null>(null);
  const {
    data: issues = [],
    error: issuesError,
    isLoading: issuesLoading,
  } = useIssues("all");

  useEffect(() => {
    const controller = new AbortController();

    fetchTags(controller.signal)
      .then((payload) => {
        setTags(payload);
      })
      .catch((caughtError) => {
        if (controller.signal.aborted) {
          return;
        }
        setError(
          caughtError instanceof Error ? caughtError.message : "Failed to load tags"
        );
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setLoading(false);
        }
      });

    return () => controller.abort();
  }, []);

  const embeddedTags = useMemo(
    () => tags.filter((tag) => Array.isArray(tag.embedding) && tag.embedding.length > 1),
    [tags]
  );

  const resolvedSelectedTagName =
    selectedTagName && embeddedTags.some((tag) => tag.name === selectedTagName)
      ? selectedTagName
      : embeddedTags[0]?.name ?? null;

  const points = useMemo(
    () => projectTags(embeddedTags),
    [embeddedTags]
  );

  const pointsByName = useMemo(
    () => new Map(points.map((point) => [point.tag.name, point])),
    [points]
  );

  const edges = useMemo(
    () => buildEdges(embeddedTags, edgeThreshold),
    [edgeThreshold, embeddedTags]
  );

  const selectedPoint = resolvedSelectedTagName
    ? pointsByName.get(resolvedSelectedTagName) ?? null
    : null;
  const selectedNeighbors = useMemo(() => {
    if (!selectedPoint) {
      return [];
    }

    return embeddedTags
      .filter((tag) => tag.name !== selectedPoint.tag.name)
      .map((tag) => ({
        name: tag.name,
        description: tag.description ?? "",
        similarity: cosineSimilarity(selectedPoint.tag.embedding, tag.embedding),
      }))
      .sort((left, right) => right.similarity - left.similarity)
      .slice(0, 6);
  }, [embeddedTags, selectedPoint]);
  const selectedMergeCandidates = useMemo(
    () => buildMergeCandidates(selectedPoint?.tag ?? null, embeddedTags),
    [embeddedTags, selectedPoint]
  );
  const selectedSpecificSuggestions = useMemo(
    () =>
      selectedPoint
        ? buildSpecificTagSuggestions(selectedPoint.tag.name, tags, issues)
        : [],
    [issues, selectedPoint, tags]
  );
  const consolidationCandidates = useMemo(
    () => buildConsolidationCandidates(tags, issues),
    [issues, tags]
  );

  const similarityMatrix = useMemo(
    () => buildSimilarityMatrix(embeddedTags),
    [embeddedTags]
  );

  return (
    <AppShell sidebar={<AppSidebar things={SECTION_LINKS} />}>
      <SiteHeader
        title="Tag Map"
        eyebrow="Tags"
        subtitle="Visualize semantic relationships between tags from stored tag embeddings."
      />

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 px-4 py-6 lg:px-6">
          <section
            id="controls"
            className="app-surface grid gap-4 p-5 lg:grid-cols-[minmax(0,1fr)_20rem]"
          >
            <div className="space-y-4">
              <div>
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Projection
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Tag positions use PCA so the layout stays stable and readable as tags change over time.
                </p>
              </div>
            </div>

            <div className="space-y-4">
              <div>
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Edge threshold
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Only show tag-to-tag links above this cosine similarity.
                </p>
              </div>
              <div className="app-subtle-surface px-4 py-3">
                <div className="flex items-center justify-between text-sm">
                  <span>Similarity</span>
                  <span className="font-medium">{Math.round(edgeThreshold * 100)}%</span>
                </div>
                <input
                  type="range"
                  min="0.45"
                  max="0.95"
                  step="0.01"
                  value={edgeThreshold}
                  onChange={(event) => setEdgeThreshold(Number(event.target.value))}
                  className="mt-3 w-full"
                />
              </div>
            </div>
          </section>

          <section
            id="projection"
            className="app-surface grid gap-6 p-5 lg:grid-cols-[minmax(0,1.5fr)_minmax(18rem,0.8fr)]"
          >
            <div>
              <div className="flex items-center justify-between gap-3">
                <div>
                  <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                    Semantic projection
                  </p>
                  <p className="mt-1 text-sm text-muted-foreground">
                    Click a tag to inspect its nearest semantic neighbors.
                  </p>
                </div>
                <Badge variant="outline" className="text-xs">
                  {embeddedTags.length} tags
                </Badge>
              </div>

              <div className="mt-4 overflow-hidden rounded-2xl border border-border/70 bg-[radial-gradient(circle_at_top,var(--glow-color)_0%,transparent_55%),linear-gradient(180deg,color-mix(in_oklab,var(--background)_94%,white_6%)_0%,color-mix(in_oklab,var(--background)_90%,var(--gradient-end)_10%)_100%)]">
                {loading ? (
                  <div className="p-6">
                    <Skeleton className="h-[34rem] w-full rounded-[1.6rem]" />
                  </div>
                ) : error ? (
                  <div className="flex h-[34rem] items-center justify-center px-6 text-sm text-destructive">
                    Failed to load tags: {error}
                  </div>
                ) : points.length === 0 ? (
                  <div className="flex h-[34rem] items-center justify-center px-6 text-sm text-muted-foreground">
                    No tag embeddings are available yet.
                  </div>
                ) : (
                  <svg viewBox="0 0 1000 760" className="h-[34rem] w-full">
                    {edges.map((edge) => {
                      const source = pointsByName.get(edge.source);
                      const target = pointsByName.get(edge.target);
                      if (!source || !target) return null;

                      return (
                        <line
                          key={`${edge.source}-${edge.target}`}
                          x1={source.x}
                          y1={source.y}
                          x2={target.x}
                          y2={target.y}
                          stroke="rgba(71, 85, 105, 0.28)"
                          strokeWidth={1 + (edge.similarity - edgeThreshold) * 6}
                        />
                      );
                    })}

                    {points.map((point) => {
                      const selected = point.tag.name === resolvedSelectedTagName;
                      return (
                        <g
                          key={point.tag.name}
                          transform={`translate(${point.x}, ${point.y})`}
                          onClick={() => setSelectedTagName(point.tag.name)}
                          className="cursor-pointer"
                        >
                          <circle
                            r={selected ? 15 : 11}
                            fill={tagColor(point.tag.name)}
                            stroke={selected ? "#0f172a" : "rgba(15, 23, 42, 0.18)"}
                            strokeWidth={selected ? 3 : 1.5}
                          />
                          <text
                            x={selected ? 22 : 18}
                            y={5}
                            fontSize="22"
                            fontWeight={selected ? "700" : "600"}
                            fill="#0f172a"
                          >
                            {point.tag.name}
                          </text>
                        </g>
                      );
                    })}
                  </svg>
                )}
              </div>
            </div>

            <div className="space-y-4">
              <div className="app-subtle-surface p-4">
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Inspector
                </p>
                {!selectedPoint ? (
                  <p className="mt-3 text-sm text-muted-foreground">
                    Select a tag to inspect its neighborhood.
                  </p>
                ) : (
                  <>
                    <div className="mt-3 flex items-start justify-between gap-3">
                      <div>
                        <p className="text-lg font-semibold">{selectedPoint.tag.name}</p>
                        <p className="mt-1 text-sm text-muted-foreground">
                          {selectedPoint.tag.description || "No description"}
                        </p>
                      </div>
                      <div className="flex items-center gap-2">
                        <span
                          className="rounded-full px-2 py-1 text-xs font-medium text-white shadow-sm"
                          style={{ backgroundColor: tagColor(selectedPoint.tag.name) }}
                        >
                          PCA
                        </span>
                        <span className="rounded-full border border-border/60 bg-background px-2.5 py-1 text-xs font-medium text-muted-foreground">
                          {isGenericBucketTag(selectedPoint.tag.name)
                            ? "Generic bucket"
                            : "Specific tag"}
                        </span>
                        <Link
                          href={tagHref(selectedPoint.tag.name)}
                          className="rounded-full border border-border/60 bg-background px-2.5 py-1 text-xs font-medium text-foreground transition-colors hover:bg-muted"
                        >
                          View tag
                        </Link>
                      </div>
                    </div>

                    <div className="mt-4 space-y-2">
                      <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                        Specificity
                      </p>
                      {isGenericBucketTag(selectedPoint.tag.name) ? (
                        <>
                          <p className="text-sm text-muted-foreground">
                            This tag is a broad bucket. Prefer a concrete subsystem or feature-area tag when one exists.
                          </p>
                          {issuesLoading ? (
                            <p className="text-sm text-muted-foreground">
                              Loading co-occurring tags...
                            </p>
                          ) : issuesError ? (
                            <p className="text-sm text-muted-foreground">
                              Could not load issue data for specificity suggestions.
                            </p>
                          ) : selectedSpecificSuggestions.length > 0 ? (
                            selectedSpecificSuggestions.map((suggestion) => (
                              <Link
                                key={suggestion.name}
                                href={tagHref(suggestion.name)}
                                className="app-subtle-surface flex items-start justify-between gap-3 px-3 py-2 transition-colors hover:bg-muted/40"
                              >
                                <div>
                                  <p className="text-sm font-medium">{suggestion.name}</p>
                                  <p className="text-xs text-muted-foreground">
                                    {suggestion.description || "No description"}
                                  </p>
                                </div>
                                <div className="text-right text-xs text-muted-foreground">
                                  <div>{Math.round(suggestion.relevance * 100)}%</div>
                                  <div>{suggestion.count} issues</div>
                                </div>
                              </Link>
                            ))
                          ) : (
                            <p className="text-sm text-muted-foreground">
                              No sharper co-occurring tags have surfaced yet.
                            </p>
                          )}
                        </>
                      ) : (
                        <p className="text-sm text-muted-foreground">
                          This already reads like a concrete application surface or subsystem tag.
                        </p>
                      )}
                    </div>

                    <div className="mt-4 space-y-2">
                      <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                        Potential merges
                      </p>
                      {selectedMergeCandidates.length > 0 ? (
                        selectedMergeCandidates.map((candidate) => (
                          <Link
                            key={candidate.name}
                            href={tagHref(candidate.name)}
                            className="app-subtle-surface flex items-start justify-between gap-3 px-3 py-2 transition-colors hover:bg-muted/40"
                          >
                            <div>
                              <p className="text-sm font-medium">{candidate.name}</p>
                              <p className="text-xs text-muted-foreground">
                                {candidate.reason}
                              </p>
                            </div>
                            <span className="text-xs font-medium tabular-nums text-muted-foreground">
                              {Math.round(candidate.similarity * 100)}%
                            </span>
                          </Link>
                        ))
                      ) : (
                        <p className="text-sm text-muted-foreground">
                          No obvious synonym or merge candidates for this tag.
                        </p>
                      )}
                    </div>

                    <div className="mt-4 space-y-2">
                      <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                        Nearest neighbors
                      </p>
                      {selectedNeighbors.map((neighbor) => (
                        <button
                          key={neighbor.name}
                          type="button"
                          onClick={() => setSelectedTagName(neighbor.name)}
                          className="app-subtle-surface flex w-full items-start justify-between gap-3 px-3 py-2 text-left hover:bg-muted/40"
                        >
                          <div>
                            <p className="text-sm font-medium">{neighbor.name}</p>
                            <p className="text-xs text-muted-foreground">
                              {neighbor.description || "No description"}
                            </p>
                          </div>
                          <span className="text-xs font-medium tabular-nums text-muted-foreground">
                            {Math.round(neighbor.similarity * 100)}%
                          </span>
                        </button>
                      ))}
                    </div>
                  </>
                )}
              </div>

              <div className="app-subtle-surface p-4">
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Notes
                </p>
                <div className="mt-3 space-y-2 text-sm text-muted-foreground">
                  <p>
                    `PCA` preserves broad global axes and tends to move less when tags change.
                  </p>
                </div>
              </div>
            </div>
          </section>

          <section
            id="consolidation"
            className="app-surface p-5"
          >
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Consolidation
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Candidate merges based on lexical variants, embedding similarity, and overlapping issue coverage.
                </p>
              </div>
              <Badge variant="outline">
                {consolidationCandidates.length} candidates
              </Badge>
            </div>

            {loading || issuesLoading ? (
              <p className="mt-4 text-sm text-muted-foreground">
                Loading tag consolidation signals...
              </p>
            ) : error || issuesError ? (
              <p className="mt-4 text-sm text-muted-foreground">
                Could not compute consolidation candidates from the current tag and issue data.
              </p>
            ) : consolidationCandidates.length === 0 ? (
              <p className="mt-4 text-sm text-muted-foreground">
                No strong consolidation candidates have surfaced yet.
              </p>
            ) : (
              <div className="mt-4 grid gap-3 lg:grid-cols-2">
                {consolidationCandidates.map((candidate) => (
                  <div
                    key={`${candidate.canonicalName}-${candidate.aliasName}`}
                    className="app-subtle-surface rounded-2xl p-4"
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div>
                        <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                          Suggested consolidation
                        </p>
                        <p className="mt-2 text-sm leading-6">
                          Consolidate{" "}
                          <Link
                            href={tagHref(candidate.aliasName)}
                            className="font-semibold text-foreground underline decoration-border/70 underline-offset-4"
                          >
                            {candidate.aliasName}
                          </Link>{" "}
                          into{" "}
                          <Link
                            href={tagHref(candidate.canonicalName)}
                            className="font-semibold text-foreground underline decoration-border/70 underline-offset-4"
                          >
                            {candidate.canonicalName}
                          </Link>
                          .
                        </p>
                        <p className="mt-2 text-xs text-muted-foreground">
                          {candidate.reason}
                        </p>
                      </div>
                      <span className="rounded-full border border-border/60 bg-background px-2.5 py-1 text-xs font-medium text-muted-foreground">
                        {Math.round(candidate.score * 100)} score
                      </span>
                    </div>

                    <div className="mt-4 grid gap-3 sm:grid-cols-2">
                      <div>
                        <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                          Canonical tag
                        </p>
                        <p className="mt-1 text-sm font-medium">{candidate.canonicalName}</p>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {candidate.canonicalDescription || "No description"}
                        </p>
                      </div>
                      <div>
                        <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                          Alias candidate
                        </p>
                        <p className="mt-1 text-sm font-medium">{candidate.aliasName}</p>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {candidate.aliasDescription || "No description"}
                        </p>
                      </div>
                    </div>

                    <div className="mt-4 flex flex-wrap gap-2 text-xs text-muted-foreground">
                      <span className="rounded-full border border-border/60 bg-background px-2.5 py-1">
                        {Math.round(candidate.similarity * 100)}% semantic similarity
                      </span>
                      <span className="rounded-full border border-border/60 bg-background px-2.5 py-1">
                        {candidate.sharedIssueCount} shared issues
                      </span>
                      <span className="rounded-full border border-border/60 bg-background px-2.5 py-1">
                        {Math.round(candidate.containment * 100)}% containment
                      </span>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </section>

          <section
            id="matrix"
            className="app-surface p-5"
          >
            <div>
              <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                Similarity matrix
              </p>
              <p className="mt-1 text-sm text-muted-foreground">
                Raw cosine similarity between tag embeddings.
              </p>
            </div>

            {similarityMatrix.names.length === 0 ? (
              <p className="mt-4 text-sm text-muted-foreground">
                No tag embeddings are available yet.
              </p>
            ) : (
              <div className="mt-4 overflow-x-auto">
                <div
                  className="grid gap-[2px]"
                  style={{
                    gridTemplateColumns: `10rem repeat(${similarityMatrix.names.length}, minmax(3rem, 1fr))`,
                  }}
                >
                  <div />
                  {similarityMatrix.names.map((name) => (
                    <div
                      key={`col-${name}`}
                      className="truncate px-2 py-1 text-center text-[10px] font-medium uppercase tracking-[0.12em] text-muted-foreground"
                    >
                      {name}
                    </div>
                  ))}

                  {similarityMatrix.names.map((rowName, rowIndex) => (
                    <Fragment key={`matrix-row-${rowName}`}>
                      <div
                        className="truncate px-2 py-2 text-sm font-medium"
                      >
                        {rowName}
                      </div>
                      {similarityMatrix.values[rowIndex].map((value, columnIndex) => (
                        <button
                          key={`${rowName}-${similarityMatrix.names[columnIndex]}`}
                          type="button"
                          onClick={() => setSelectedTagName(rowName)}
                          className="aspect-square rounded-md text-[11px] font-medium tabular-nums text-slate-950"
                          style={{
                            backgroundColor: heatmapColor(value),
                            opacity: rowIndex === columnIndex ? 1 : 0.92,
                          }}
                          title={`${rowName} / ${similarityMatrix.names[columnIndex]}: ${Math.round(value * 100)}%`}
                        >
                          {Math.round(value * 100)}
                        </button>
                      ))}
                    </Fragment>
                  ))}
                </div>
              </div>
            )}
          </section>
        </div>
      </div>
    </AppShell>
  );
}

function projectTags(tags: TagRecord[]): TagPoint[] {
  if (tags.length === 0) {
    return [];
  }

  if (tags.length === 1) {
    return [{ tag: tags[0], x: 500, y: 380 }];
  }

  const vectors = tags.map((tag) => normalizeVector(tag.embedding));
  const rawPoints = computePCA(vectors);
  const normalized = normalizePoints(rawPoints, 120, 880, 100, 660);

  return tags.map((tag, index) => ({
    tag,
    x: normalized[index][0],
    y: normalized[index][1],
  }));
}

function computePCA(vectors: number[][]): number[][] {
  const dimensions = vectors[0]?.length ?? 0;
  if (dimensions < 2) {
    return fallbackPoints(vectors.length);
  }

  const centered = centerVectors(vectors);
  const covariance = buildCovariance(centered);
  const first = powerIteration(covariance);
  const deflated = deflateMatrix(covariance, first.vector, first.eigenvalue);
  const second = powerIteration(deflated);

  if (first.vector.length === 0 || second.vector.length === 0) {
    return fallbackPoints(vectors.length);
  }

  return centered.map((vector) => [dot(vector, first.vector), dot(vector, second.vector)]);
}

function buildEdges(tags: TagRecord[], threshold: number): TagEdge[] {
  const edges: TagEdge[] = [];
  for (let i = 0; i < tags.length; i += 1) {
    for (let j = i + 1; j < tags.length; j += 1) {
      const similarity = cosineSimilarity(tags[i].embedding, tags[j].embedding);
      if (similarity >= threshold) {
        edges.push({
          source: tags[i].name,
          target: tags[j].name,
          similarity,
        });
      }
    }
  }

  edges.sort((left, right) => right.similarity - left.similarity);
  return edges;
}

function buildSimilarityMatrix(tags: TagRecord[]) {
  const names = tags.map((tag) => tag.name);
  const values = tags.map((left) =>
    tags.map((right) => cosineSimilarity(left.embedding, right.embedding))
  );
  return { names, values };
}

function centerVectors(vectors: number[][]): number[][] {
  const dimensions = vectors[0]?.length ?? 0;
  const means = new Array(dimensions).fill(0);
  for (const vector of vectors) {
    for (let i = 0; i < dimensions; i += 1) {
      means[i] += vector[i] ?? 0;
    }
  }
  for (let i = 0; i < dimensions; i += 1) {
    means[i] /= vectors.length;
  }

  return vectors.map((vector) => vector.map((value, index) => value - means[index]));
}

function buildCovariance(vectors: number[][]): number[][] {
  const dimensions = vectors[0]?.length ?? 0;
  const covariance = Array.from({ length: dimensions }, () =>
    new Array(dimensions).fill(0)
  );
  const divisor = Math.max(1, vectors.length - 1);

  for (const vector of vectors) {
    for (let row = 0; row < dimensions; row += 1) {
      for (let column = row; column < dimensions; column += 1) {
        covariance[row][column] += vector[row] * vector[column];
      }
    }
  }

  for (let row = 0; row < dimensions; row += 1) {
    for (let column = row; column < dimensions; column += 1) {
      covariance[row][column] /= divisor;
      covariance[column][row] = covariance[row][column];
    }
  }

  return covariance;
}

function powerIteration(matrix: number[][]) {
  const dimensions = matrix.length;
  if (dimensions === 0) {
    return { vector: [] as number[], eigenvalue: 0 };
  }

  let vector: number[] = new Array(dimensions)
    .fill(0)
    .map((_, index) => (index % 2 === 0 ? 1 : -1));
  vector = normalizeVector(vector);

  for (let iteration = 0; iteration < 80; iteration += 1) {
    const next = multiplyMatrixVector(matrix, vector);
    const normalized = normalizeVector(next);
    if (normalized.every((value) => value === 0)) {
      break;
    }
    vector = normalized;
  }

  const projected = multiplyMatrixVector(matrix, vector);
  return {
    vector,
    eigenvalue: dot(vector, projected),
  };
}

function deflateMatrix(matrix: number[][], vector: number[], eigenvalue: number) {
  return matrix.map((row, rowIndex) =>
    row.map((value, columnIndex) => value - eigenvalue * vector[rowIndex] * vector[columnIndex])
  );
}

function multiplyMatrixVector(matrix: number[][], vector: number[]) {
  return matrix.map((row) => dot(row, vector));
}

function dot(left: number[], right: number[]) {
  let total = 0;
  for (let i = 0; i < left.length; i += 1) {
    total += (left[i] ?? 0) * (right[i] ?? 0);
  }
  return total;
}

function normalizeVector(vector: number[]) {
  const magnitude = Math.sqrt(vector.reduce((sum, value) => sum + value * value, 0));
  if (magnitude === 0) {
    return vector.map(() => 0);
  }
  return vector.map((value) => value / magnitude);
}

function normalizePoints(
  points: number[][],
  minX: number,
  maxX: number,
  minY: number,
  maxY: number
) {
  const xs = points.map((point) => point[0]);
  const ys = points.map((point) => point[1]);
  const xBounds = extent(xs);
  const yBounds = extent(ys);

  return points.map(([x, y]) => [
    scaleIntoRange(x, xBounds[0], xBounds[1], minX, maxX),
    scaleIntoRange(y, yBounds[0], yBounds[1], maxY, minY),
  ]);
}

function extent(values: number[]) {
  let min = values[0] ?? 0;
  let max = values[0] ?? 0;
  for (const value of values) {
    min = Math.min(min, value);
    max = Math.max(max, value);
  }
  return [min, max] as const;
}

function scaleIntoRange(
  value: number,
  minValue: number,
  maxValue: number,
  minTarget: number,
  maxTarget: number
) {
  if (maxValue === minValue) {
    return (minTarget + maxTarget) / 2;
  }
  return minTarget + ((value - minValue) / (maxValue - minValue)) * (maxTarget - minTarget);
}

function fallbackPoints(count: number) {
  return Array.from({ length: count }, (_, index) => {
    const angle = (Math.PI * 2 * index) / count;
    return [Math.cos(angle), Math.sin(angle)];
  });
}

function tagColor(name: string) {
  let hash = 0;
  for (let index = 0; index < name.length; index += 1) {
    hash = (hash * 31 + name.charCodeAt(index)) >>> 0;
  }
  return `hsl(${hash % 360} 70% 46%)`;
}

function heatmapColor(value: number) {
  const clamped = Math.max(-1, Math.min(1, value));
  const positive = clamped >= 0;
  const intensity = Math.round(Math.abs(clamped) * 100);
  return positive
    ? `hsl(168 65% ${92 - intensity * 0.38}%)`
    : `hsl(8 80% ${92 - intensity * 0.32}%)`;
}
