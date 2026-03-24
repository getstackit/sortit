"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import { uiAPIURL } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";

type TagScore = {
  tag: string;
  relevance: number;
  suggested?: boolean;
  description?: string;
};

type ModelInfo = {
  provider: string;
  model: string;
};

type CandidateTag = {
  name: string;
  description?: string;
  hint?: boolean;
  sources: string[];
  similarity?: number;
};

type CandidateSet = {
  mode: string;
  tags: CandidateTag[];
};

type IssueAnalysis = {
  tags: TagScore[];
  candidateSet: CandidateSet;
  embedding: {
    dimensions: number;
    preview: number[];
    chunkCount: number;
    estimatedTokenCount: number;
    pooledFromChunks: boolean;
  };
  tagger: ModelInfo;
  embedder: ModelInfo;
  comparedIssueCount: number;
  averageIssueSimilarity: number;
  similarIssues: Array<{
    id: string;
    raw: string;
    tags: string[];
    similarity: number;
  }>;
};

type InvalidateMapProjectionResponse = {
  invalidated: boolean;
};

type LowR2Issue = {
  id: string;
  raw: string;
  status: "open" | "closed";
  r2: number;
  tags: string[];
};

type ReviewIssue = {
  id: string;
  raw: string;
  status: "open" | "closed";
  r2: number;
  tagScores: TagScore[];
  diagnosis: string[];
};

type ReviewLowAlignment = {
  issueId: string;
  issueRaw: string;
  issueStatus: "open" | "closed";
  issueR2: number;
  tagScore: TagScore;
  alignment: number;
};

type ResidualCandidateTag = {
  tag: string;
  similarity: number;
  assigned: boolean;
};

type ReviewResidualMiss = {
  issueId: string;
  issueRaw: string;
  issueStatus: "open" | "closed";
  issueR2: number;
  tagScores: TagScore[];
  candidateTags: ResidualCandidateTag[];
};

type ReviewQueue = {
  lowR2Issues: ReviewIssue[];
  lowAlignmentTags: ReviewLowAlignment[];
  residualMisses: ReviewResidualMiss[];
};

type FactorWeights = {
  factorWeight: number;
  residualWeight: number;
  aggregateR2: number;
  issueCount: number;
  decomposedCount: number;
  decomposed: boolean;
  lowR2Issues: LowR2Issue[];
  reviewQueue: ReviewQueue;
};

const SECTION_LINKS = [
  { id: "prompt", title: "Prompt" },
  { id: "factor-weights", title: "Factor weights" },
  { id: "candidates", title: "Candidates" },
  { id: "tags", title: "Tags" },
  { id: "embedding", title: "Embedding" },
  { id: "similarity", title: "Similarity" },
  { id: "json", title: "JSON" },
];

const DEFAULT_TEXT = `Safari export crashes when I try to download a PDF from the issue detail view on iPad. It hangs for a second, then the sheet disappears and nothing is saved.`;

function formatFloat(value: number) {
  return value.toFixed(3);
}

function formatCandidateMode(value: string) {
  switch (value) {
    case "retrieval-shortlist":
      return "Retrieval shortlist";
    case "full-catalog":
      return "Full catalog";
    case "explicit-only":
      return "Explicit only";
    default:
      return value;
  }
}

function TagScoreList({ scores }: { scores: TagScore[] }) {
  if (scores.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">No persisted tag scores.</p>
    );
  }

  return (
    <div className="flex flex-wrap gap-2">
      {scores.map((score) => (
        <div
          key={`${score.tag}-${score.relevance}`}
          className="rounded-2xl border border-border/70 bg-background/70 px-3 py-2 text-sm"
        >
          <div className="flex items-center gap-2">
            <span className="font-medium">{score.tag}</span>
            <span className="text-muted-foreground">
              {formatFloat(score.relevance)}
            </span>
            {score.suggested && (
              <span className="rounded-full border border-amber-300 bg-amber-100 px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.14em] text-amber-900">
                Suggested
              </span>
            )}
          </div>
          {score.description && (
            <p className="mt-1 max-w-72 text-xs leading-5 text-muted-foreground">
              {score.description}
            </p>
          )}
        </div>
      ))}
    </div>
  );
}

async function parseDebugResponse(response: Response) {
  const raw = await response.text();
  const contentType = response.headers.get("content-type") ?? "";
  const isJSON = contentType.includes("application/json");

  if (isJSON) {
    return JSON.parse(raw) as IssueAnalysis | { error?: string };
  }

  if (!response.ok) {
    throw new Error(raw.trim() || `Request failed with ${response.status}`);
  }

  throw new Error("Backend returned a non-JSON response.");
}

export default function DebugPage() {
  const [text, setText] = useState(DEFAULT_TEXT);
  const [tags, setTags] = useState("");
  const [candidateMode, setCandidateMode] = useState<"retrieval-shortlist" | "full-catalog">("retrieval-shortlist");
  const [result, setResult] = useState<IssueAnalysis | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [invalidateLoading, setInvalidateLoading] = useState(false);
  const [invalidateError, setInvalidateError] = useState<string | null>(null);
  const [invalidateMessage, setInvalidateMessage] = useState<string | null>(null);
  const [rescoreLoading, setRescoreLoading] = useState(false);
  const [rescoreError, setRescoreError] = useState<string | null>(null);
  const [rescoreMessage, setRescoreMessage] = useState<string | null>(null);
  const [factorWeights, setFactorWeights] = useState<FactorWeights | null>(null);
  const [factorWeightsLoading, setFactorWeightsLoading] = useState(false);
  const [factorWeightsError, setFactorWeightsError] = useState<string | null>(null);
  const [showOnlyOpenLowR2Issues, setShowOnlyOpenLowR2Issues] = useState(false);
  const [reviewActionMessage, setReviewActionMessage] = useState<string | null>(null);
  const [reviewActionError, setReviewActionError] = useState<string | null>(null);
  const [reEnrichingIssueIDs, setReEnrichingIssueIDs] = useState<Record<string, boolean>>({});

  const topTags = result?.tags.slice(0, 8) ?? [];
  const embeddingPreview = result?.embedding.preview ?? [];
  const similarIssues = result?.similarIssues ?? [];
  const filteredLowR2Issues = useMemo(() => {
    const items = factorWeights?.lowR2Issues ?? [];
    if (!showOnlyOpenLowR2Issues) {
      return items;
    }
    return items.filter((issue) => issue.status === "open");
  }, [factorWeights?.lowR2Issues, showOnlyOpenLowR2Issues]);
  const reviewQueue = factorWeights?.reviewQueue;

  async function analyze() {
    const trimmed = text.trim();
    if (!trimmed) {
      setError("Issue text is required.");
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const response = await fetch(uiAPIURL("/debug/issues/analyze"), {
        method: "POST",
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          text: trimmed,
          tags: tags
            .split(",")
            .map((value) => value.trim())
            .filter(Boolean),
          candidateMode,
        }),
      });

      const payload = await parseDebugResponse(response);

      if (!response.ok) {
        throw new Error(
          "error" in payload && payload.error
            ? payload.error
            : `Request failed with ${response.status}`
        );
      }

      setResult(payload as IssueAnalysis);
    } catch (caughtError) {
      setResult(null);
      setError(
        caughtError instanceof Error
          ? caughtError.message
          : "Unknown analysis error"
      );
    } finally {
      setLoading(false);
    }
  }

  async function invalidateMapProjection() {
    setInvalidateLoading(true);
    setInvalidateError(null);
    setInvalidateMessage(null);

    try {
      const response = await fetch(
        uiAPIURL("/debug/map-projection/invalidate"),
        {
          method: "POST",
          credentials: "include",
        }
      );

      const payload =
        (await parseDebugResponse(
          response
        )) as InvalidateMapProjectionResponse | { error?: string };

      if (!response.ok) {
        throw new Error(
          "error" in payload && payload.error
            ? payload.error
            : `Request failed with ${response.status}`
        );
      }

      if (!("invalidated" in payload) || !payload.invalidated) {
        throw new Error("Backend did not confirm invalidation.");
      }

      setInvalidateMessage("Derived corpus invalidated. It will rebuild on next read.");
    } catch (caughtError) {
      setInvalidateError(
        caughtError instanceof Error
          ? caughtError.message
          : "Unknown invalidation error"
      );
    } finally {
      setInvalidateLoading(false);
    }
  }

  async function rescoreTagSpecificity() {
    setRescoreLoading(true);
    setRescoreError(null);
    setRescoreMessage(null);

    try {
      const response = await fetch(uiAPIURL("/debug/tags/rescore"), {
        method: "POST",
        credentials: "include",
      });

      const payload = (await parseDebugResponse(response)) as
        | { rescored?: boolean }
        | { error?: string };

      if (!response.ok) {
        throw new Error(
          "error" in payload && payload.error
            ? payload.error
            : `Request failed with ${response.status}`
        );
      }

      if (!("rescored" in payload) || !payload.rescored) {
        throw new Error("Backend did not confirm rescoring.");
      }

      setRescoreMessage("All tag specificity scores recalculated.");
    } catch (caughtError) {
      setRescoreError(
        caughtError instanceof Error
          ? caughtError.message
          : "Unknown rescoring error"
      );
    } finally {
      setRescoreLoading(false);
    }
  }

  async function loadFactorWeights() {
    setFactorWeightsLoading(true);
    setFactorWeightsError(null);

    try {
      const response = await fetch(uiAPIURL("/debug/factor-weights"), {
        credentials: "include",
      });

      if (!response.ok) {
        throw new Error(`Request failed with ${response.status}`);
      }

      const payload = (await response.json()) as FactorWeights;
      setFactorWeights(payload);
    } catch (caughtError) {
      setFactorWeightsError(
        caughtError instanceof Error
          ? caughtError.message
          : "Failed to load factor weights"
      );
    } finally {
      setFactorWeightsLoading(false);
    }
  }

  async function reEnrichIssueFromReview(issueID: string) {
    setReviewActionMessage(null);
    setReviewActionError(null);
    setReEnrichingIssueIDs((current) => ({ ...current, [issueID]: true }));

    try {
      const response = await fetch(uiAPIURL(`/issues/${encodeURIComponent(issueID)}/re-enrich`), {
        method: "POST",
        credentials: "include",
      });
      const payload = await parseDebugResponse(response);

      if (!response.ok) {
        throw new Error(
          "error" in payload && payload.error
            ? payload.error
            : `Request failed with ${response.status}`
        );
      }

      setReviewActionMessage(`${issueID} queued for re-enrichment.`);
      await loadFactorWeights();
    } catch (caughtError) {
      setReviewActionError(
        caughtError instanceof Error
          ? caughtError.message
          : "Failed to re-enrich issue"
      );
    } finally {
      setReEnrichingIssueIDs((current) => {
        const next = { ...current };
        delete next[issueID];
        return next;
      });
    }
  }

  return (
    <AppShell sidebar={<AppSidebar things={SECTION_LINKS} />}>
      <SiteHeader
        title="Debug analyzer"
        eyebrow="Debug"
        subtitle="Paste an issue, inspect tags, inspect embedding output."
      />

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 px-4 py-6 lg:px-6">
          <section
            id="prompt"
            className="app-surface grid gap-6 p-5 lg:grid-cols-[minmax(0,1.4fr)_minmax(20rem,0.8fr)]"
          >
            <div className="space-y-3">
              <div>
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Issue text
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  This page calls the backend debug endpoint directly so you can
                  iterate on model output without touching persistence.
                </p>
              </div>

              <textarea
                value={text}
                onChange={(event) => setText(event.target.value)}
                placeholder="Paste an issue here..."
                className="min-h-64 w-full rounded-xl border border-input/80 bg-background/72 px-3 py-3 text-sm leading-6 outline-none transition-colors focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
              />
            </div>

            <div className="space-y-4">
              <div>
                <label
                  htmlFor="tags"
                  className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground"
                >
                  Tags override
                </label>
                <p className="mt-1 text-sm text-muted-foreground">
                  Optional comma-separated tags. Leave empty to use the backend
                  canonical tag set.
                </p>
              </div>

              <Input
                id="tags"
                value={tags}
                onChange={(event) => setTags(event.target.value)}
                placeholder="bug, safari, export"
              />

              <div className="space-y-2">
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Candidate mode
                </p>
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant={candidateMode === "retrieval-shortlist" ? "default" : "outline"}
                    onClick={() => setCandidateMode("retrieval-shortlist")}
                    disabled={loading}
                  >
                    Retrieval shortlist
                  </Button>
                  <Button
                    variant={candidateMode === "full-catalog" ? "default" : "outline"}
                    onClick={() => setCandidateMode("full-catalog")}
                    disabled={loading}
                  >
                    Full catalog
                  </Button>
                </div>
                <p className="text-sm text-muted-foreground">
                  Compare the production shortlist against the full-catalog fallback.
                  If you provide explicit tags above, the backend will switch to an
                  explicit-only candidate set.
                </p>
              </div>

              <div className="flex flex-wrap items-center gap-3">
                <Button onClick={analyze} disabled={loading}>
                  {loading ? "Analyzing..." : "Analyze issue"}
                </Button>
                <Button
                  variant="outline"
                  onClick={invalidateMapProjection}
                  disabled={loading || invalidateLoading}
                >
                  {invalidateLoading
                    ? "Invalidating corpus..."
                    : "Invalidate map projection"}
                </Button>
                <Button
                  variant="outline"
                  onClick={rescoreTagSpecificity}
                  disabled={rescoreLoading}
                >
                  {rescoreLoading
                    ? "Rescoring tags..."
                    : "Rescore tag specificity"}
                </Button>
                <Button
                  variant="outline"
                  onClick={() => {
                    setText(DEFAULT_TEXT);
                    setTags("");
                    setCandidateMode("retrieval-shortlist");
                  }}
                  disabled={loading}
                >
                  Reset example
                </Button>
              </div>

              {error && (
                <div className="app-status-warning">
                  {error}
                </div>
              )}

              {invalidateError && (
                <div className="app-status-warning">
                  {invalidateError}
                </div>
              )}

              {invalidateMessage && (
                <div className="app-status-success">
                  {invalidateMessage}
                </div>
              )}

              {rescoreError && (
                <div className="app-status-warning">
                  {rescoreError}
                </div>
              )}

              {rescoreMessage && (
                <div className="app-status-success">
                  {rescoreMessage}
                </div>
              )}

              {result && (
                <div className="app-status-success">
                  <p className="font-medium">Analysis complete</p>
                  <p className="mt-1">
                    Tagger: {result.tagger.provider} / {result.tagger.model}
                  </p>
                  <p className="mt-1">
                    Embedder: {result.embedder.provider} /{" "}
                    {result.embedder.model}
                  </p>
                  <p className="mt-1">
                    Candidate set: {formatCandidateMode(result.candidateSet.mode)} /{" "}
                    {result.candidateSet.tags.length} tags
                  </p>
                </div>
              )}
            </div>
          </section>

          <section
            id="factor-weights"
            className="app-surface p-5"
          >
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Factor decomposition weights
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Data-driven blend weights computed from the variance explained
                  by the tag-factor model vs. residual embedding similarity.
                </p>
              </div>
              <Button
                variant="outline"
                onClick={loadFactorWeights}
                disabled={factorWeightsLoading}
              >
                {factorWeightsLoading ? "Loading..." : "Load weights"}
              </Button>
            </div>

            {factorWeightsError && (
              <div className="mt-4 app-status-warning">{factorWeightsError}</div>
            )}

            {reviewActionError && (
              <div className="mt-4 app-status-warning">{reviewActionError}</div>
            )}

            {reviewActionMessage && (
              <div className="mt-4 app-status-success">{reviewActionMessage}</div>
            )}

            {factorWeights && (
              <div className="mt-4 space-y-4">
                <div className="grid gap-3 lg:grid-cols-5">
                  <div className="app-subtle-surface px-4 py-3">
                    <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">
                      Aggregate R²
                    </p>
                    <p className="mt-1 text-lg font-medium">
                      {(factorWeights.aggregateR2 * 100).toFixed(1)}%
                    </p>
                  </div>
                  <div className="app-subtle-surface px-4 py-3">
                    <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">
                      Factor weight
                    </p>
                    <p className="mt-1 text-lg font-medium">
                      {factorWeights.factorWeight.toFixed(3)}
                    </p>
                  </div>
                  <div className="app-subtle-surface px-4 py-3">
                    <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">
                      Residual weight
                    </p>
                    <p className="mt-1 text-lg font-medium">
                      {factorWeights.residualWeight.toFixed(3)}
                    </p>
                  </div>
                  <div className="app-subtle-surface px-4 py-3">
                    <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">
                      Decomposed
                    </p>
                    <p className="mt-1 text-lg font-medium">
                      {factorWeights.decomposed
                        ? `${factorWeights.decomposedCount} / ${factorWeights.issueCount}`
                        : "No (fallback)"}
                    </p>
                  </div>
                  <div className="app-subtle-surface px-4 py-3">
                    <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">
                      Total issues
                    </p>
                    <p className="mt-1 text-lg font-medium">
                      {factorWeights.issueCount}
                    </p>
                  </div>
                </div>

                {factorWeights.lowR2Issues?.length > 0 && (
                  <div>
                    <div className="flex items-start justify-between gap-3">
                      <div>
                        <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
                          Low R² issues
                        </p>
                        <p className="mt-1 text-sm text-muted-foreground">
                          Issues poorly explained by the tag factor model. Candidates
                          for new tags or re-classification.
                        </p>
                      </div>
                      <label className="flex items-center gap-2 text-[11px] text-muted-foreground">
                        <Switch
                          checked={showOnlyOpenLowR2Issues}
                          onCheckedChange={setShowOnlyOpenLowR2Issues}
                          aria-label="Only show open low R² issues"
                        />
                        <span>Only open issues</span>
                      </label>
                    </div>
                    <div className="mt-3 space-y-2">
                      {filteredLowR2Issues.map((issue) => (
                        <div
                          key={issue.id}
                          className="app-subtle-surface px-4 py-3"
                        >
                          <div className="flex items-start justify-between gap-4">
                            <div className="min-w-0">
                              <p className="text-sm font-medium">{issue.id}</p>
                              <p className="mt-1 truncate text-sm text-muted-foreground">
                                {issue.raw}
                              </p>
                              {issue.tags.length > 0 && (
                                <p className="mt-1 text-xs text-muted-foreground">
                                  Tags: {issue.tags.join(", ")}
                                </p>
                              )}
                            </div>
                            <div className="flex shrink-0 items-center gap-2">
                              <Badge
                                variant="outline"
                                className="text-xs text-foreground"
                              >
                                {issue.status}
                              </Badge>
                              <Badge
                                variant="outline"
                                className="text-xs text-foreground"
                              >
                                R² {(issue.r2 * 100).toFixed(1)}%
                              </Badge>
                            </div>
                          </div>
                        </div>
                      ))}
                      {filteredLowR2Issues.length === 0 && (
                        <div className="app-subtle-surface px-4 py-3 text-sm text-muted-foreground">
                          No open low R² issues in the current result set.
                        </div>
                      )}
                    </div>
                  </div>
                )}

                {reviewQueue && (
                  <div className="space-y-4">
                    <div>
                      <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
                        Review queue
                      </p>
                      <p className="mt-1 text-sm text-muted-foreground">
                        Open issues that look worth re-enriching based on low R²,
                        low-alignment assigned tags, or strong residual-near missed tags.
                      </p>
                    </div>

                    <div className="grid gap-3 lg:grid-cols-3">
                      <div className="app-subtle-surface px-4 py-3">
                        <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">
                          Open low R²
                        </p>
                        <p className="mt-1 text-lg font-medium">
                          {reviewQueue.lowR2Issues.length}
                        </p>
                      </div>
                      <div className="app-subtle-surface px-4 py-3">
                        <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">
                          Low alignment
                        </p>
                        <p className="mt-1 text-lg font-medium">
                          {reviewQueue.lowAlignmentTags.length}
                        </p>
                      </div>
                      <div className="app-subtle-surface px-4 py-3">
                        <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">
                          Residual misses
                        </p>
                        <p className="mt-1 text-lg font-medium">
                          {reviewQueue.residualMisses.length}
                        </p>
                      </div>
                    </div>

                    <div className="space-y-4">
                      <div>
                        <p className="text-sm font-semibold">Open low R² issues</p>
                        <div className="mt-3 space-y-3">
                          {reviewQueue.lowR2Issues.length === 0 ? (
                            <div className="app-subtle-surface px-4 py-3 text-sm text-muted-foreground">
                              No open low R² issues in the current review queue.
                            </div>
                          ) : (
                            reviewQueue.lowR2Issues.map((issue) => (
                              <div key={issue.id} className="app-subtle-surface px-4 py-4">
                                <div className="flex items-start justify-between gap-4">
                                  <div className="min-w-0">
                                    <p className="text-sm font-medium">
                                      <Link href={`/issues/${issue.id}`} className="hover:underline">
                                        {issue.id}
                                      </Link>
                                    </p>
                                    <p className="mt-1 text-sm text-muted-foreground">
                                      {issue.raw}
                                    </p>
                                  </div>
                                  <div className="flex shrink-0 items-center gap-2">
                                    <Badge variant="outline" className="text-xs text-foreground">
                                      {issue.status}
                                    </Badge>
                                    <Badge variant="outline" className="text-xs text-foreground">
                                      R² {(issue.r2 * 100).toFixed(1)}%
                                    </Badge>
                                    <Button
                                      variant="outline"
                                      disabled={Boolean(reEnrichingIssueIDs[issue.id])}
                                      onClick={() => reEnrichIssueFromReview(issue.id)}
                                    >
                                      {reEnrichingIssueIDs[issue.id] ? "Queueing..." : "Re-enrich"}
                                    </Button>
                                  </div>
                                </div>
                                <div className="mt-3">
                                  <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                                    Current tag scores
                                  </p>
                                  <div className="mt-2">
                                    <TagScoreList scores={issue.tagScores} />
                                  </div>
                                </div>
                                {issue.diagnosis.length > 0 && (
                                  <ul className="mt-3 list-disc space-y-1 pl-4 text-xs text-muted-foreground">
                                    {issue.diagnosis.map((item, index) => (
                                      <li key={index}>{item}</li>
                                    ))}
                                  </ul>
                                )}
                              </div>
                            ))
                          )}
                        </div>
                      </div>

                      <div>
                        <p className="text-sm font-semibold">High-relevance, low-alignment tags</p>
                        <div className="mt-3 space-y-3">
                          {reviewQueue.lowAlignmentTags.length === 0 ? (
                            <div className="app-subtle-surface px-4 py-3 text-sm text-muted-foreground">
                              No open issues currently have strongly suspicious assigned tags.
                            </div>
                          ) : (
                            reviewQueue.lowAlignmentTags.map((item) => (
                              <div
                                key={`${item.issueId}-${item.tagScore.tag}`}
                                className="app-subtle-surface px-4 py-4"
                              >
                                <div className="flex items-start justify-between gap-4">
                                  <div className="min-w-0">
                                    <p className="text-sm font-medium">
                                      <Link href={`/issues/${item.issueId}`} className="hover:underline">
                                        {item.issueId}
                                      </Link>
                                    </p>
                                    <p className="mt-1 text-sm text-muted-foreground">
                                      {item.issueRaw}
                                    </p>
                                  </div>
                                  <div className="flex shrink-0 items-center gap-2">
                                    <Badge variant="outline" className="text-xs text-foreground">
                                      R² {(item.issueR2 * 100).toFixed(1)}%
                                    </Badge>
                                    <Badge variant="outline" className="text-xs text-foreground">
                                      Alignment {formatFloat(item.alignment)}
                                    </Badge>
                                    <Button
                                      variant="outline"
                                      disabled={Boolean(reEnrichingIssueIDs[item.issueId])}
                                      onClick={() => reEnrichIssueFromReview(item.issueId)}
                                    >
                                      {reEnrichingIssueIDs[item.issueId] ? "Queueing..." : "Re-enrich"}
                                    </Button>
                                  </div>
                                </div>
                                <div className="mt-3">
                                  <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                                    Flagged tag
                                  </p>
                                  <div className="mt-2">
                                    <TagScoreList scores={[item.tagScore]} />
                                  </div>
                                </div>
                              </div>
                            ))
                          )}
                        </div>
                      </div>

                      <div>
                        <p className="text-sm font-semibold">Residual-near missed tags</p>
                        <div className="mt-3 space-y-3">
                          {reviewQueue.residualMisses.length === 0 ? (
                            <div className="app-subtle-surface px-4 py-3 text-sm text-muted-foreground">
                              No strong residual-near missed catalog tags right now.
                            </div>
                          ) : (
                            reviewQueue.residualMisses.map((item) => (
                              <div
                                key={item.issueId}
                                className="app-subtle-surface px-4 py-4"
                              >
                                <div className="flex items-start justify-between gap-4">
                                  <div className="min-w-0">
                                    <p className="text-sm font-medium">
                                      <Link href={`/issues/${item.issueId}`} className="hover:underline">
                                        {item.issueId}
                                      </Link>
                                    </p>
                                    <p className="mt-1 text-sm text-muted-foreground">
                                      {item.issueRaw}
                                    </p>
                                  </div>
                                  <div className="flex shrink-0 items-center gap-2">
                                    <Badge variant="outline" className="text-xs text-foreground">
                                      R² {(item.issueR2 * 100).toFixed(1)}%
                                    </Badge>
                                    <Button
                                      variant="outline"
                                      disabled={Boolean(reEnrichingIssueIDs[item.issueId])}
                                      onClick={() => reEnrichIssueFromReview(item.issueId)}
                                    >
                                      {reEnrichingIssueIDs[item.issueId] ? "Queueing..." : "Re-enrich"}
                                    </Button>
                                  </div>
                                </div>
                                <div className="mt-3">
                                  <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                                    Current tag scores
                                  </p>
                                  <div className="mt-2">
                                    <TagScoreList scores={item.tagScores} />
                                  </div>
                                </div>
                                <div className="mt-3">
                                  <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                                    Candidate catalog tags
                                  </p>
                                  <div className="mt-2 flex flex-wrap gap-2">
                                    {item.candidateTags.map((candidate) => (
                                      <div
                                        key={`${item.issueId}-${candidate.tag}`}
                                        className="rounded-2xl border border-border/70 bg-background/70 px-3 py-2 text-sm"
                                      >
                                        <div className="flex items-center gap-2">
                                          <span className="font-medium">{candidate.tag}</span>
                                          <span className="text-muted-foreground">
                                            {formatFloat(candidate.similarity)}
                                          </span>
                                        </div>
                                      </div>
                                    ))}
                                  </div>
                                </div>
                              </div>
                            ))
                          )}
                        </div>
                      </div>
                    </div>
                  </div>
                )}
              </div>
            )}

            {!factorWeights && !factorWeightsError && (
              <p className="mt-4 text-sm text-muted-foreground">
                Click &ldquo;Load weights&rdquo; to compute the current factor
                decomposition weights from the issue corpus.
              </p>
            )}
          </section>

          <section
            id="candidates"
            className="app-surface p-5"
          >
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Candidate set
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  The exact taxonomy passed to the model, including how each tag entered the set.
                </p>
              </div>
              {result && (
                <div className="flex items-center gap-2">
                  <Badge variant="outline" className="text-xs">
                    {formatCandidateMode(result.candidateSet.mode)}
                  </Badge>
                  <Badge variant="outline" className="text-xs">
                    {result.candidateSet.tags.length} tags
                  </Badge>
                </div>
              )}
            </div>

            {!result && (
              <p className="mt-4 text-sm text-muted-foreground">
                Run an analysis to inspect the candidate taxonomy.
              </p>
            )}

            {result && (
              <div className="mt-4 flex flex-wrap gap-2">
                {result.candidateSet.tags.map((tag) => (
                  <div
                    key={tag.name}
                    className="app-subtle-surface rounded-2xl px-3 py-3 text-sm"
                  >
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium">{tag.name}</span>
                      {typeof tag.similarity === "number" && (
                        <span className="text-muted-foreground">
                          {formatFloat(tag.similarity)}
                        </span>
                      )}
                      {tag.hint && (
                        <span className="rounded-full border border-sky-300 bg-sky-100 px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.14em] text-sky-900">
                          Hint
                        </span>
                      )}
                    </div>
                    <div className="mt-2 flex flex-wrap gap-1">
                      {tag.sources.map((source) => (
                        <span
                          key={`${tag.name}-${source}`}
                          className="rounded-full border border-border/70 bg-background/70 px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.14em] text-muted-foreground"
                        >
                          {source}
                        </span>
                      ))}
                    </div>
                    {tag.description && (
                      <p className="mt-2 max-w-80 text-xs leading-5 text-muted-foreground">
                        {tag.description}
                      </p>
                    )}
                  </div>
                ))}
              </div>
            )}
          </section>

          <section
            id="tags"
            className="app-surface p-5"
          >
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Top tags
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Highest relevance scores returned by the backend.
                </p>
              </div>
              {result && (
                <Badge variant="outline" className="text-xs">
                  {result.tags.length} tags
                </Badge>
              )}
            </div>

            {!result && (
              <p className="mt-4 text-sm text-muted-foreground">
                Run an analysis to inspect tag scores.
              </p>
            )}

            {result && (
              <div className="mt-4 flex flex-wrap gap-2">
                {topTags.map((tag) => (
                  <div
                    key={tag.tag}
                    className="app-subtle-surface rounded-2xl px-3 py-2 text-sm"
                  >
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{tag.tag}</span>
                      <span className="text-muted-foreground">
                        {formatFloat(tag.relevance)}
                      </span>
                      {tag.suggested && (
                        <span className="rounded-full border border-amber-300 bg-amber-100 px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.14em] text-amber-900">
                          Suggested
                        </span>
                      )}
                    </div>
                    {tag.description && (
                      <p className="mt-1 max-w-72 text-xs leading-5 text-muted-foreground">
                        {tag.description}
                      </p>
                    )}
                  </div>
                ))}
              </div>
            )}
          </section>

          <section
            id="embedding"
            className="app-surface p-5"
          >
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Embedding
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Chunking and pooling metadata for the final embedding.
                </p>
              </div>
              {result && (
                <Badge variant="outline" className="text-xs">
                  {result.embedding.dimensions} dims
                </Badge>
              )}
            </div>

            {!result && (
              <p className="mt-4 text-sm text-muted-foreground">
                Run an analysis to inspect the embedding vector.
              </p>
            )}

            {result && (
              <div className="mt-4 grid gap-3 lg:grid-cols-[minmax(0,16rem)_minmax(0,16rem)_minmax(0,1fr)]">
                <div className="app-subtle-surface px-4 py-3">
                  <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">
                    Estimated tokens
                  </p>
                  <p className="mt-1 text-lg font-medium">
                    {result.embedding.estimatedTokenCount}
                  </p>
                </div>
                <div className="app-subtle-surface px-4 py-3">
                  <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">
                    Chunks
                  </p>
                  <p className="mt-1 text-lg font-medium">
                    {result.embedding.chunkCount}
                    {result.embedding.pooledFromChunks ? " pooled" : " direct"}
                  </p>
                </div>
                <div className="app-subtle-surface px-4 py-3">
                  <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">
                    Preview
                  </p>
                  <div className="mt-2 flex flex-wrap gap-2">
                    {embeddingPreview.map((value, index) => (
                      <span
                        key={index}
                        className="rounded-full bg-muted px-2 py-1 font-mono text-xs"
                      >
                        d{index}: {formatFloat(value)}
                      </span>
                    ))}
                  </div>
                </div>
              </div>
            )}
          </section>

          <section
            id="similarity"
            className="app-surface p-5"
          >
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Embedding similarity
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Cosine similarity between the analyzed issue embedding and stored issue embeddings.
                </p>
              </div>
              {result && (
                <Badge variant="outline" className="text-xs">
                  {result.comparedIssueCount} compared
                </Badge>
              )}
            </div>

            {!result && (
              <p className="mt-4 text-sm text-muted-foreground">
                Run an analysis to inspect embedding-based similarity.
              </p>
            )}

            {result && (
              <div className="mt-4 space-y-4">
                <div className="app-subtle-surface px-4 py-3">
                  <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">
                    Average similarity
                  </p>
                  <p className="mt-1 text-lg font-medium">
                    {result.comparedIssueCount === 0
                      ? "No stored issues"
                      : `${Math.round(result.averageIssueSimilarity * 100)}%`}
                  </p>
                </div>

                {similarIssues.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    No stored issue embeddings available to compare against.
                  </p>
                ) : (
                  <div className="space-y-2">
                    {similarIssues.map((issue) => (
                      <div
                        key={issue.id}
                        className="app-subtle-surface px-4 py-3"
                      >
                        <div className="flex items-start justify-between gap-4">
                          <div>
                            <p className="text-sm font-medium">{issue.id}</p>
                            <p className="mt-1 text-sm text-muted-foreground">
                              {issue.raw}
                            </p>
                            {issue.tags.length > 0 && (
                              <p className="mt-2 text-xs text-muted-foreground">
                                Tags: {issue.tags.join(", ")}
                              </p>
                            )}
                          </div>
                          <Badge variant="outline" className="text-xs text-foreground">
                            {Math.round(issue.similarity * 100)}%
                          </Badge>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}
          </section>

          <section
            id="json"
            className="app-surface p-5"
          >
            <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
              Raw JSON
            </p>
            <p className="mt-1 text-sm text-muted-foreground">
              Full response payload from the backend.
            </p>

            <pre className="mt-4 overflow-x-auto rounded-xl border border-border/70 bg-muted/40 p-4 text-xs leading-6 text-foreground/90">
              {result ? JSON.stringify(result, null, 2) : "{\n  \n}"}
            </pre>
          </section>
        </div>
      </div>
    </AppShell>
  );
}
