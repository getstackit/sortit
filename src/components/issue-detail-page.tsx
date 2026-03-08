"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  CheckIcon,
  Clock3Icon,
  CopyIcon,
  HashIcon,
  Link2Icon,
} from "lucide-react";
import { AppShell } from "@/components/app-shell";
import {
  IssueMapCanvas,
  type IssueMapCanvasCluster,
  type IssueMapCanvasLine,
  type IssueMapCanvasNode,
} from "@/components/issue-map-canvas";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import { Button, buttonVariants } from "@/components/ui/button";
import { apiURL } from "@/lib/api";
import { cn } from "@/lib/utils";
import {
  closeIssue,
  fetchIssue,
  reopenIssue,
  type IssueRecord,
} from "@/lib/issues";

type TagRelevance = {
  tag: string;
  relevance: number;
};

type MapIssue = {
  id: string;
  raw: string;
  status: IssueRecord["status"];
  tags: TagRelevance[];
  x: number;
  y: number;
};

type MapEdge = {
  source: string;
  target: string;
  similarity: number;
};

type MapCluster = {
  label: string;
  centerX: number;
  centerY: number;
  radius: number;
  color: string;
};

type MapData = {
  issues: MapIssue[];
  edges: MapEdge[];
  clusters: MapCluster[];
};

type SemanticNeighbor = {
  issue: MapIssue;
  similarity: number;
};

type ClusterNeighbor = {
  issue: MapIssue;
  distance: number;
};

const EMPTY_MAP_ISSUES: MapIssue[] = [];
const EMPTY_MAP_EDGES: MapEdge[] = [];
const EMPTY_MAP_CLUSTERS: MapCluster[] = [];

function formatDateTime(value: string) {
  return new Date(value).toLocaleString();
}

function formatRelativeTime(value: string) {
  const timestamp = new Date(value).getTime();
  const diffSeconds = Math.round((timestamp - Date.now()) / 1000);
  const formatter = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });

  const ranges: Array<[Intl.RelativeTimeFormatUnit, number]> = [
    ["year", 60 * 60 * 24 * 365],
    ["month", 60 * 60 * 24 * 30],
    ["week", 60 * 60 * 24 * 7],
    ["day", 60 * 60 * 24],
    ["hour", 60 * 60],
    ["minute", 60],
  ];

  for (const [unit, seconds] of ranges) {
    if (Math.abs(diffSeconds) >= seconds || unit === "minute") {
      return formatter.format(Math.round(diffSeconds / seconds), unit);
    }
  }

  return "just now";
}

function formatIssueTitle(raw: string, maxLength = 84) {
  const normalized = raw.replace(/\s+/g, " ").trim();
  return normalized.length > maxLength
    ? `${normalized.slice(0, maxLength).trimEnd()}...`
    : normalized;
}

function looksLikeStructuredText(text: string) {
  return (
    text.includes("\n") ||
    text.includes("Error") ||
    text.includes("Exception") ||
    text.includes(" at ") ||
    /^\s*(import|const|let|var|def|class)\b/m.test(text)
  );
}

function splitParagraphs(text: string) {
  return text
    .split(/\n{2,}/)
    .map((paragraph) => paragraph.trim())
    .filter(Boolean);
}

function statusClasses(status: IssueRecord["status"]) {
  return status === "closed"
    ? "bg-slate-200 text-slate-700"
    : "bg-emerald-100 text-emerald-700";
}

function dominantTag(tags: TagRelevance[]) {
  if (tags.length === 0) {
    return null;
  }

  return tags.reduce((best, current) =>
    current.relevance > best.relevance ? current : best
  ).tag;
}

function distanceBetween(
  left: Pick<MapIssue, "x" | "y">,
  right: Pick<MapIssue, "x" | "y">
) {
  return Math.hypot(left.x - right.x, left.y - right.y);
}

function clusterContainsIssue(cluster: MapCluster, issue: MapIssue) {
  return (
    Math.hypot(issue.x - cluster.centerX, issue.y - cluster.centerY) <= cluster.radius
  );
}

function projectPoint(x: number, y: number, width: number, height: number) {
  const padding = 18;

  return {
    x: padding + x * (width - padding * 2),
    y: height - padding - y * (height - padding * 2),
  };
}

async function fetchMapData(signal?: AbortSignal): Promise<MapData> {
  const response = await fetch(apiURL("/api/v1/map?status=all&edgeThreshold=0.4"), {
    cache: "no-store",
    signal,
  });

  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`);
  }

  return (await response.json()) as MapData;
}

async function copyText(value: string) {
  if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value);
    return;
  }

  if (typeof document === "undefined") {
    throw new Error("Clipboard unavailable");
  }

  const input = document.createElement("textarea");
  input.value = value;
  input.setAttribute("readonly", "");
  input.style.position = "absolute";
  input.style.left = "-9999px";
  document.body.appendChild(input);
  input.select();

  const succeeded = document.execCommand("copy");
  document.body.removeChild(input);

  if (!succeeded) {
    throw new Error("Clipboard unavailable");
  }
}

export function IssueDetailPage({ issueID }: { issueID: string }) {
  const [issue, setIssue] = useState<IssueRecord | null>(null);
  const [mapData, setMapData] = useState<MapData | null>(null);
  const [loading, setLoading] = useState(true);
  const [issueError, setIssueError] = useState<string | null>(null);
  const [mapError, setMapError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [copyState, setCopyState] = useState<
    "idle" | "text-copied" | "link-copied" | "error"
  >("idle");
  const [statusPending, setStatusPending] = useState(false);

  useEffect(() => {
    const controller = new AbortController();

    setLoading(true);
    setIssueError(null);
    setMapError(null);
    setActionError(null);

    Promise.allSettled([fetchIssue(issueID, controller.signal), fetchMapData(controller.signal)])
      .then(([issueResult, mapResult]) => {
        if (issueResult.status === "fulfilled") {
          setIssue(issueResult.value);
          setIssueError(null);
        } else if (!controller.signal.aborted) {
          const message =
            issueResult.reason instanceof Error
              ? issueResult.reason.message
              : "Unknown backend error";
          setIssue(null);
          setIssueError(message);
        }

        if (mapResult.status === "fulfilled") {
          setMapData(mapResult.value);
          setMapError(null);
        } else if (!controller.signal.aborted) {
          const message =
            mapResult.reason instanceof Error
              ? mapResult.reason.message
              : "Unknown backend error";
          setMapData(null);
          setMapError(message);
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setLoading(false);
        }
      });

    return () => controller.abort();
  }, [issueID]);

  useEffect(() => {
    if (copyState === "idle") {
      return;
    }

    const timeout = window.setTimeout(() => {
      setCopyState("idle");
    }, 2000);

    return () => window.clearTimeout(timeout);
  }, [copyState]);

  const handleCopyIssue = useCallback(async () => {
    if (!issue) {
      return;
    }

    try {
      await copyText(issue.raw);
      setCopyState("text-copied");
      setActionError(null);
    } catch {
      setCopyState("error");
      setActionError("Clipboard unavailable.");
    }
  }, [issue]);

  const handleCopyLink = useCallback(async () => {
    if (typeof window === "undefined") {
      return;
    }

    try {
      await copyText(window.location.href);
      setCopyState("link-copied");
      setActionError(null);
    } catch {
      setCopyState("error");
      setActionError("Clipboard unavailable.");
    }
  }, []);

  const handleStatusChange = useCallback(async () => {
    if (!issue || statusPending) {
      return;
    }

    setStatusPending(true);

    try {
      const updated =
        issue.status === "closed"
          ? await reopenIssue(issue.id)
          : await closeIssue(issue.id);

      setIssue(updated);
      setActionError(null);
    } catch (caughtError) {
      const message =
        caughtError instanceof Error
          ? caughtError.message
          : "Unknown backend error";
      setActionError(message);
    } finally {
      setStatusPending(false);
    }
  }, [issue, statusPending]);

  const shortcuts = useMemo(
    () =>
      issue
        ? [
            {
              key: "y",
              description: "Copy issue text",
              action: () => void handleCopyIssue(),
            },
            {
              key: "x",
              description:
                issue.status === "closed"
                  ? "Reopen this issue"
                  : "Close this issue",
              action: () => void handleStatusChange(),
            },
          ]
        : [],
    [handleCopyIssue, handleStatusChange, issue]
  );

  const mapIssues = mapData?.issues ?? EMPTY_MAP_ISSUES;
  const mapEdges = mapData?.edges ?? EMPTY_MAP_EDGES;
  const mapClusters = mapData?.clusters ?? EMPTY_MAP_CLUSTERS;

  const mapIssueIndex = useMemo(
    () => new Map(mapIssues.map((entry) => [entry.id, entry])),
    [mapIssues]
  );

  const currentMapIssue = issue ? mapIssueIndex.get(issue.id) ?? null : null;

  const currentClusters = useMemo(() => {
    if (!currentMapIssue) {
      return [];
    }

    return mapClusters.filter((cluster) => clusterContainsIssue(cluster, currentMapIssue));
  }, [currentMapIssue, mapClusters]);

  const semanticNeighbors = useMemo<SemanticNeighbor[]>(() => {
    if (!currentMapIssue) {
      return [];
    }

    const neighbors = mapEdges
      .flatMap((edge) => {
        if (edge.source === currentMapIssue.id) {
          return [{ id: edge.target, similarity: edge.similarity }];
        }
        if (edge.target === currentMapIssue.id) {
          return [{ id: edge.source, similarity: edge.similarity }];
        }
        return [];
      })
      .map((neighbor) => {
        const relatedIssue = mapIssueIndex.get(neighbor.id);
        return relatedIssue
          ? { issue: relatedIssue, similarity: neighbor.similarity }
          : null;
      })
      .filter((neighbor): neighbor is SemanticNeighbor => neighbor !== null);

    neighbors.sort((left, right) => right.similarity - left.similarity);
    return neighbors.slice(0, 5);
  }, [currentMapIssue, mapEdges, mapIssueIndex]);

  const clusterNeighbors = useMemo<ClusterNeighbor[]>(() => {
    if (!currentMapIssue) {
      return [];
    }

    const clusterScopedIssues =
      currentClusters.length > 0
        ? mapIssues.filter(
            (candidate) =>
              candidate.id !== currentMapIssue.id &&
              currentClusters.some((cluster) => clusterContainsIssue(cluster, candidate))
          )
        : mapIssues.filter((candidate) => candidate.id !== currentMapIssue.id);

    const ranked = clusterScopedIssues.map((candidate) => ({
      issue: candidate,
      distance: distanceBetween(currentMapIssue, candidate),
    }));

    ranked.sort((left, right) => left.distance - right.distance);
    return ranked.slice(0, 5);
  }, [currentClusters, currentMapIssue, mapIssues]);

  const semanticNeighborIds = useMemo(
    () => new Set(semanticNeighbors.map((neighbor) => neighbor.issue.id)),
    [semanticNeighbors]
  );

  const clusterNeighborIds = useMemo(
    () => new Set(clusterNeighbors.map((neighbor) => neighbor.issue.id)),
    [clusterNeighbors]
  );

  const miniMapClusters = useMemo<IssueMapCanvasCluster[]>(() => {
    return mapClusters.map((cluster) => {
      const center = projectPoint(cluster.centerX, cluster.centerY, 320, 220);

      return {
        key: `${cluster.label}-${cluster.centerX}-${cluster.centerY}`,
        cx: center.x,
        cy: center.y,
        radius: Math.max(cluster.radius * (320 - 36), 10),
        fill: cluster.color,
        fillOpacity: currentClusters.includes(cluster) ? 0.18 : 0.08,
        stroke: cluster.color,
        strokeOpacity: currentClusters.includes(cluster) ? 0.5 : 0.18,
        strokeWidth: currentClusters.includes(cluster) ? 1.6 : 1,
      };
    });
  }, [currentClusters, mapClusters]);

  const miniMapEdges = useMemo<IssueMapCanvasLine[]>(() => {
    if (!currentMapIssue) {
      return [];
    }

    return semanticNeighbors.map(({ issue: neighbor, similarity }) => {
      const from = projectPoint(currentMapIssue.x, currentMapIssue.y, 320, 220);
      const to = projectPoint(neighbor.x, neighbor.y, 320, 220);

      return {
        key: `${currentMapIssue.id}-${neighbor.id}`,
        x1: from.x,
        y1: from.y,
        x2: to.x,
        y2: to.y,
        stroke: "#f59e0b",
        strokeOpacity: 0.22 + similarity * 0.45,
        strokeWidth: 1 + similarity * 1.6,
      };
    });
  }, [currentMapIssue, semanticNeighbors]);

  const miniMapNodes = useMemo<IssueMapCanvasNode[]>(() => {
    return mapIssues.map((entry) => {
      const point = projectPoint(entry.x, entry.y, 320, 220);
      const isCurrent = entry.id === currentMapIssue?.id;
      const isSemantic = semanticNeighborIds.has(entry.id);
      const isCluster = clusterNeighborIds.has(entry.id);

      return {
        key: entry.id,
        cx: point.x,
        cy: point.y,
        radius: isCurrent ? 6 : isSemantic || isCluster ? 4 : 2.5,
        fill: isCurrent
          ? "#0f172a"
          : isSemantic && isCluster
            ? "#c2410c"
            : isSemantic
              ? "#f59e0b"
              : isCluster
                ? "#2563eb"
                : "#94a3b8",
        fillOpacity: isCurrent ? 1 : isSemantic || isCluster ? 0.95 : 0.35,
        stroke: isCurrent ? "#ffffff" : undefined,
        strokeWidth: isCurrent ? 2 : 0,
      };
    });
  }, [clusterNeighborIds, currentMapIssue, mapIssues, semanticNeighborIds]);

  const timeline = useMemo(() => {
    if (!issue) {
      return [];
    }

    const entries = [
      {
        label: "Created",
        value: formatDateTime(issue.createdAt),
        meta: `${formatRelativeTime(issue.createdAt)} by ${issue.createdBy}`,
      },
    ];

    if (issue.status === "closed" && issue.closedAt) {
      entries.push({
        label: "Closed",
        value: formatDateTime(issue.closedAt),
        meta: `${formatRelativeTime(issue.closedAt)}${issue.closedBy ? ` by ${issue.closedBy}` : ""}`,
      });
    }

    return entries;
  }, [issue]);

  return (
    <AppShell
      sidebar={<AppSidebar showThingsSection={false} />}
      shortcuts={shortcuts}
    >
      <SiteHeader
        title={issue ? formatIssueTitle(issue.raw, 68) : issueID}
        eyebrow={issue?.id ?? "Issue"}
        subtitle={issue ? `${issue.createdBy} · ${formatRelativeTime(issue.createdAt)}` : undefined}
        meta={
          issue ? (
            <span
              className={cn(
                "rounded-full px-2 py-0.5 text-[11px] font-medium",
                statusClasses(issue.status)
              )}
            >
              {issue.status === "closed" ? "Closed" : "Open"}
            </span>
          ) : null
        }
        actions={
          <>
            <Link
              href="/"
              className="rounded-md border border-border px-3 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              All issues
            </Link>
            {issue && (
              <Link
                href={`/map?issue=${encodeURIComponent(issue.id)}`}
                className="rounded-md border border-border px-3 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
              >
                Open on map
              </Link>
            )}
          </>
        }
      />

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-6xl flex-col gap-6 px-4 py-6 lg:px-6">
          {loading && (
            <div className="rounded-2xl border border-border/60 bg-card p-5 text-sm text-muted-foreground">
              Loading issue...
            </div>
          )}

          {!loading && issueError && (
            <div className="rounded-2xl border border-amber-200 bg-amber-50 p-5 text-sm text-amber-900">
              {issueError === "issue not found"
                ? `No issue exists for "${issueID}".`
                : `Issue backend unavailable: ${issueError}`}
            </div>
          )}

          {!loading && issue && (
            <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_18rem]">
              <div className="space-y-6">
                {actionError && (
                  <div className="rounded-2xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900">
                    {actionError}
                  </div>
                )}

                <section className="rounded-[1.75rem] border border-border/60 bg-card p-6 shadow-sm">
                  <div className="flex flex-wrap items-start justify-between gap-4">
                    <div className="space-y-2">
                      <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                        Raw report
                      </p>
                      <h2 className="max-w-3xl text-2xl leading-tight font-semibold tracking-tight text-balance">
                        {formatIssueTitle(issue.raw, 140)}
                      </h2>
                      <p className="text-sm text-muted-foreground">
                        Original issue text preserved as entered.
                      </p>
                    </div>
                    <div
                      className={cn(
                        "rounded-full px-3 py-1 text-xs font-medium",
                        statusClasses(issue.status)
                      )}
                    >
                      {issue.status === "closed" ? "Closed" : "Open"}
                    </div>
                  </div>

                  <div className="mt-6 rounded-[1.5rem] border border-border/50 bg-muted/20 p-5">
                    {looksLikeStructuredText(issue.raw) ? (
                      <pre className="overflow-x-auto whitespace-pre-wrap break-words font-mono text-[13px] leading-6 text-foreground/85">
                        {issue.raw}
                      </pre>
                    ) : (
                      <div className="space-y-4 text-[15px] leading-7 text-foreground/90">
                        {splitParagraphs(issue.raw).map((paragraph, index) => (
                          <p key={index}>{paragraph}</p>
                        ))}
                      </div>
                    )}
                  </div>
                </section>

                <section className="rounded-[1.75rem] border border-border/60 bg-card p-6 shadow-sm">
                  <div className="flex flex-wrap items-start justify-between gap-4">
                    <div>
                      <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                        Map context
                      </p>
                      <h3 className="mt-2 text-lg font-semibold tracking-tight">
                        Cluster position and semantic edges
                      </h3>
                      <p className="mt-1 text-sm text-muted-foreground">
                        The mini-map uses the same coordinates and edge graph as the full map.
                      </p>
                    </div>
                    <span className="rounded-full bg-muted px-2.5 py-1 text-[11px] font-medium text-muted-foreground">
                      {currentClusters.length > 0
                        ? `${currentClusters.length} cluster${currentClusters.length === 1 ? "" : "s"}`
                        : "No cluster hit"}
                    </span>
                  </div>

                  {mapError && (
                    <div className="mt-5 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
                      Map context unavailable: {mapError}
                    </div>
                  )}

                  {!mapError && !currentMapIssue && (
                    <div className="mt-5 rounded-xl border border-dashed border-border/80 px-4 py-6 text-sm text-muted-foreground">
                      This issue is not present in the current map payload yet.
                    </div>
                  )}

                  {!mapError && currentMapIssue && (
                    <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,1.2fr)_minmax(0,0.8fr)]">
                      <div className="rounded-[1.5rem] border border-border/50 bg-muted/20 p-4">
                        <IssueMapCanvas
                          width={320}
                          height={220}
                          className="h-auto w-full"
                          role="img"
                          aria-label={`Mini map centered on ${currentMapIssue.id}`}
                          background={
                            <rect
                              x="0"
                              y="0"
                              width="320"
                              height="220"
                              rx="18"
                              fill="currentColor"
                              className="text-background"
                            />
                          }
                          clusters={miniMapClusters}
                          edges={miniMapEdges}
                          nodes={miniMapNodes}
                        />

                        <div className="mt-4 flex flex-wrap gap-2 text-[11px] text-muted-foreground">
                          <span className="inline-flex items-center gap-1 rounded-full bg-background px-2 py-1">
                            <span className="size-2 rounded-full bg-slate-900" />
                            Current issue
                          </span>
                          <span className="inline-flex items-center gap-1 rounded-full bg-background px-2 py-1">
                            <span className="size-2 rounded-full bg-blue-600" />
                            Clustered nearby
                          </span>
                          <span className="inline-flex items-center gap-1 rounded-full bg-background px-2 py-1">
                            <span className="size-2 rounded-full bg-amber-500" />
                            Semantic neighbor
                          </span>
                        </div>
                      </div>

                      <div className="grid gap-4">
                        <div className="rounded-[1.25rem] border border-border/50 bg-muted/20 p-4">
                          <div className="flex items-start justify-between gap-3">
                            <div>
                              <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                                Clustered nearby
                              </p>
                              <p className="mt-1 text-sm text-muted-foreground">
                                Nearest issues in the same local cluster when possible.
                              </p>
                            </div>
                            <span className="rounded-full bg-background px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
                              {clusterNeighbors.length}
                            </span>
                          </div>

                          <div className="mt-4 space-y-2">
                            {clusterNeighbors.length > 0 ? (
                              clusterNeighbors.map(({ issue: neighbor }) => (
                                <Link
                                  key={neighbor.id}
                                  href={`/issues/${neighbor.id}`}
                                  className="group flex items-center gap-3 rounded-xl bg-background px-3 py-2 transition-colors hover:bg-white"
                                >
                                  <span className="size-2.5 rounded-full bg-blue-600" />
                                  <div className="min-w-0 flex-1">
                                    <p className="truncate text-xs font-medium">{neighbor.id}</p>
                                    <p className="truncate text-[11px] text-muted-foreground">
                                      {formatIssueTitle(neighbor.raw, 52)}
                                    </p>
                                  </div>
                                  <span className="text-[10px] uppercase tracking-[0.14em] text-muted-foreground">
                                    {dominantTag(neighbor.tags) ?? "issue"}
                                  </span>
                                </Link>
                              ))
                            ) : (
                              <p className="text-sm text-muted-foreground">
                                No nearby cluster matches yet.
                              </p>
                            )}
                          </div>
                        </div>

                        <div className="rounded-[1.25rem] border border-border/50 bg-muted/20 p-4">
                          <div className="flex items-start justify-between gap-3">
                            <div>
                              <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                                Semantic neighbors
                              </p>
                              <p className="mt-1 text-sm text-muted-foreground">
                                Strongest embedding edges connected to this issue.
                              </p>
                            </div>
                            <span className="rounded-full bg-background px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
                              {semanticNeighbors.length}
                            </span>
                          </div>

                          <div className="mt-4 space-y-2">
                            {semanticNeighbors.length > 0 ? (
                              semanticNeighbors.map(({ issue: neighbor, similarity }) => (
                                <Link
                                  key={neighbor.id}
                                  href={`/issues/${neighbor.id}`}
                                  className="group flex items-center gap-3 rounded-xl bg-background px-3 py-2 transition-colors hover:bg-white"
                                >
                                  <span className="size-2.5 rounded-full bg-amber-500" />
                                  <div className="min-w-0 flex-1">
                                    <p className="truncate text-xs font-medium">{neighbor.id}</p>
                                    <p className="truncate text-[11px] text-muted-foreground">
                                      {formatIssueTitle(neighbor.raw, 52)}
                                    </p>
                                  </div>
                                  <span className="text-[11px] font-medium tabular-nums text-muted-foreground">
                                    {(similarity * 100).toFixed(0)}%
                                  </span>
                                </Link>
                              ))
                            ) : (
                              <p className="text-sm text-muted-foreground">
                                No semantic edges for this issue yet.
                              </p>
                            )}
                          </div>
                        </div>
                      </div>
                    </div>
                  )}
                </section>
              </div>

              <aside className="space-y-4 lg:sticky lg:top-6 lg:self-start">
                <section className="rounded-[1.5rem] border border-border/60 bg-card p-5 shadow-sm">
                  <div className="flex items-center gap-2">
                    <HashIcon className="size-4 text-muted-foreground" />
                    <h3 className="text-sm font-semibold">Summary</h3>
                  </div>

                  <dl className="mt-4 space-y-4 text-sm">
                    <div className="space-y-1">
                      <dt className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                        Status
                      </dt>
                      <dd>
                        <span
                          className={cn(
                            "inline-flex rounded-full px-2.5 py-1 text-xs font-medium",
                            statusClasses(issue.status)
                          )}
                        >
                          {issue.status === "closed" ? "Closed" : "Open"}
                        </span>
                      </dd>
                    </div>

                    <div className="space-y-1">
                      <dt className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                        Tags
                      </dt>
                      <dd className="flex flex-wrap gap-1.5">
                        {issue.tags.length > 0 ? (
                          issue.tags.map((tag) => (
                            <span
                              key={tag}
                              className="rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-foreground"
                            >
                              {tag}
                            </span>
                          ))
                        ) : (
                          <span className="text-sm text-muted-foreground">
                            None
                          </span>
                        )}
                      </dd>
                    </div>
                  </dl>
                </section>

                <section className="rounded-[1.5rem] border border-border/60 bg-card p-5 shadow-sm">
                  <div className="flex items-center gap-2">
                    <Clock3Icon className="size-4 text-muted-foreground" />
                    <h3 className="text-sm font-semibold">Timeline</h3>
                  </div>

                  <div className="mt-4 space-y-4">
                    {timeline.map((entry) => (
                      <div
                        key={entry.label}
                        className="rounded-xl border border-border/50 bg-muted/20 p-3"
                      >
                        <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                          {entry.label}
                        </p>
                        <p className="mt-1 text-sm font-medium">{entry.value}</p>
                        <p className="mt-1 text-[11px] text-muted-foreground">
                          {entry.meta}
                        </p>
                      </div>
                    ))}
                  </div>
                </section>

                <section className="rounded-[1.5rem] border border-border/60 bg-card p-5 shadow-sm">
                  <div className="flex items-center gap-2">
                    <Link2Icon className="size-4 text-muted-foreground" />
                    <h3 className="text-sm font-semibold">Actions</h3>
                  </div>

                  <div className="mt-4 grid gap-2">
                    <Button
                      type="button"
                      aria-keyshortcuts="X"
                      variant={issue.status === "closed" ? "outline" : "default"}
                      onClick={() => void handleStatusChange()}
                      disabled={statusPending}
                    >
                      {statusPending
                        ? issue.status === "closed"
                          ? "Reopening..."
                          : "Closing..."
                        : issue.status === "closed"
                          ? "Reopen issue"
                          : "Close issue"}
                    </Button>

                    <Button
                      type="button"
                      aria-keyshortcuts="Y"
                      variant="outline"
                      onClick={() => void handleCopyIssue()}
                    >
                      {copyState === "text-copied" ? (
                        <CheckIcon aria-hidden="true" />
                      ) : (
                        <CopyIcon aria-hidden="true" />
                      )}
                      {copyState === "text-copied" ? "Copied text" : "Copy text"}
                    </Button>

                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => void handleCopyLink()}
                    >
                      {copyState === "link-copied" ? (
                        <CheckIcon aria-hidden="true" />
                      ) : (
                        <Link2Icon aria-hidden="true" />
                      )}
                      {copyState === "link-copied" ? "Copied link" : "Copy link"}
                    </Button>

                    <Link
                      href={`/map?issue=${encodeURIComponent(issue.id)}`}
                      className={buttonVariants({ variant: "outline" })}
                    >
                      Open on map
                    </Link>
                  </div>

                  <p className="mt-4 text-[11px] text-muted-foreground">
                    Keyboard shortcuts: <span className="font-medium text-foreground">X</span>{" "}
                    toggles issue status, <span className="font-medium text-foreground">Y</span>{" "}
                    copies the raw report.
                  </p>
                </section>
              </aside>
            </div>
          )}
        </div>
      </div>
    </AppShell>
  );
}
