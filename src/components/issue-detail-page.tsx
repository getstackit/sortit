"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { motion } from "motion/react";
import {
  ArrowUpCircleIcon,
  CheckIcon,
  CopyIcon,
  HashIcon,
  Link2Icon,
  MessageSquareMoreIcon,
  SparklesIcon,
  UserIcon,
} from "lucide-react";
import { useAuth } from "@/components/auth-provider";
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
import type {
  MapCluster,
  MapEdge,
  MapIssue,
} from "@/features/map/types";
import { useIssue, useIssueMapData } from "@/hooks/use-issues";
import {
  assignIssue,
  closeIssue,
  reopenIssue,
  refineIssue,
  progressIssue,
  type IssueLinkRecord,
  type IssuePostKind,
  type IssueOperationRecord,
  type IssuePostRecord,
  type IssueRecord,
} from "@/lib/issues";
import { cn } from "@/lib/utils";
import { TagRelevanceBars } from "@/components/tag-relevance-bars";
import { entityStyle } from "@/lib/entity-colors";
import { tagHref } from "@/lib/tags";

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

function compareIssueStatus(left: { status: IssueRecord["status"] }, right: { status: IssueRecord["status"] }) {
  if (left.status === right.status) {
    return 0;
  }

  return left.status === "open" ? -1 : 1;
}

function postKind(post: IssuePostRecord): IssuePostKind {
  if (post.kind) {
    return post.kind;
  }
  return post.sequence === 1 ? "report" : "refinement";
}

function formatLinkType(type: IssueLinkRecord["type"]) {
  switch (type) {
    case "parent_of":
      return "Parent of";
    case "child_of":
      return "Child of";
    case "merged_into":
      return "Merged into";
    case "derived_from":
      return "Derived from";
    case "duplicate_of":
      return "Duplicate of";
    case "related_to":
    default:
      return "Related to";
  }
}

function formatOperationKind(kind: IssueOperationRecord["kind"]) {
  switch (kind) {
    case "split":
      return "Split";
    case "combine":
      return "Combine";
    case "link":
    default:
      return "Link";
  }
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

function fallbackDiscussion(issue: IssueRecord | null | undefined): IssuePostRecord[] {
  if (!issue) {
    return [];
  }

  if (issue.discussion && issue.discussion.length > 0) {
    return issue.discussion;
  }

  return [
    {
      id: `${issue.id}-post-000001`,
      issueId: issue.id,
      raw: issue.raw,
      createdBy: issue.createdBy,
      createdAt: issue.createdAt,
      sequence: 1,
    },
  ];
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

function DiscussionBody({ text }: { text: string }) {
  if (looksLikeStructuredText(text)) {
    return (
      <pre className="overflow-x-auto whitespace-pre-wrap break-words font-mono text-[13px] leading-6 text-foreground/85">
        {text}
      </pre>
    );
  }

  return (
    <div className="space-y-4 text-[15px] leading-7 text-foreground/90">
      {splitParagraphs(text).map((paragraph, index) => (
        <p key={index}>{paragraph}</p>
      ))}
    </div>
  );
}

export function IssueDetailPage({ issueID }: { issueID: string }) {
  const { data: issue, error: issueError, isLoading: loading, mutate: mutateIssue } = useIssue(issueID);
  const { data: mapData, error: mapError, mutate: mutateMap } = useIssueMapData();
  const [actionError, setActionError] = useState<string | null>(null);
  const [copyState, setCopyState] = useState<
    "idle" | "text-copied" | "link-copied" | "error"
  >("idle");
  const [statusPending, setStatusPending] = useState(false);
  const [refineInput, setRefineInput] = useState("");
  const [refinePending, setRefinePending] = useState(false);
  const [postMode, setPostMode] = useState<"refinement" | "progress">("refinement");
  const [progressPending, setProgressPending] = useState(false);
  const { user } = useAuth();
  const [assignEditing, setAssignEditing] = useState(false);
  const [assignInput, setAssignInput] = useState("");
  const [assignPending, setAssignPending] = useState(false);

  useEffect(() => {
    if (copyState === "idle") {
      return;
    }

    const timeout = window.setTimeout(() => {
      setCopyState("idle");
    }, 2000);

    return () => window.clearTimeout(timeout);
  }, [copyState]);

  const discussion = useMemo(() => fallbackDiscussion(issue), [issue]);

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

      mutateIssue(updated, { revalidate: false });
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
  }, [issue, statusPending, mutateIssue]);

  const handleRefine = useCallback(async () => {
    if (!issue || refinePending) {
      return;
    }

    const raw = refineInput.trim();
    if (!raw) {
      return;
    }

    setRefinePending(true);

    try {
      const updated = await refineIssue(issue.id, { raw });
      mutateIssue(updated, { revalidate: false });
      setRefineInput("");
      setActionError(null);

      mutateMap();
    } catch (caughtError) {
      const message =
        caughtError instanceof Error
          ? caughtError.message
          : "Unknown backend error";
      setActionError(message);
    } finally {
      setRefinePending(false);
    }
  }, [issue, refineInput, refinePending, mutateIssue, mutateMap]);

  const handleProgress = useCallback(async () => {
    if (!issue || progressPending) {
      return;
    }

    const raw = refineInput.trim();
    if (!raw) {
      return;
    }

    setProgressPending(true);

    try {
      const updated = await progressIssue(issue.id, { raw });
      mutateIssue(updated, { revalidate: false });
      setRefineInput("");
      setActionError(null);
    } catch (caughtError) {
      const message =
        caughtError instanceof Error
          ? caughtError.message
          : "Unknown backend error";
      setActionError(message);
    } finally {
      setProgressPending(false);
    }
  }, [issue, refineInput, progressPending, mutateIssue]);

  const handleSubmit = useCallback(() => {
    if (postMode === "progress") {
      return handleProgress();
    }
    return handleRefine();
  }, [postMode, handleProgress, handleRefine]);

  const handleAssign = useCallback(async (value: string) => {
    if (!issue || assignPending) {
      return;
    }

    setAssignPending(true);
    try {
      const updated = await assignIssue(issue.id, { assignedTo: value.trim() });
      mutateIssue(updated, { revalidate: false });
      setAssignEditing(false);
      setActionError(null);
    } catch (caughtError) {
      const message = caughtError instanceof Error ? caughtError.message : "Unknown backend error";
      setActionError(message);
    } finally {
      setAssignPending(false);
    }
  }, [issue, assignPending, mutateIssue]);

  const shortcuts = useMemo(
    () =>
      issue
        ? [
            {
              key: "y",
              description: "Copy issue summary",
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

    neighbors.sort((left, right) => {
      const statusOrder = compareIssueStatus(left.issue, right.issue);
      if (statusOrder !== 0) {
        return statusOrder;
      }
      return right.similarity - left.similarity;
    });
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

    ranked.sort((left, right) => {
      const statusOrder = compareIssueStatus(left.issue, right.issue);
      if (statusOrder !== 0) {
        return statusOrder;
      }
      return left.distance - right.distance;
    });
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

  const refinementIndices = useMemo(() => {
    const indices = new Map<string, number>();
    let count = 0;
    for (const post of discussion) {
      if (postKind(post) === "refinement") {
        count++;
        indices.set(post.id, count);
      }
    }
    return indices;
  }, [discussion]);

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

  const relationshipGroups = useMemo(() => {
    const groups = new Map<string, IssueLinkRecord[]>();
    for (const link of issue?.links ?? []) {
      const key = link.type;
      const current = groups.get(key) ?? [];
      current.push(link);
      groups.set(key, current);
    }

    return [...groups.entries()].map(([type, links]) => ({
      type,
      label: formatLinkType(type as IssueLinkRecord["type"]),
      links: [...links].sort((left, right) => {
        const leftIssue = left.relatedIssue;
        const rightIssue = right.relatedIssue;
        if (leftIssue && rightIssue) {
          const statusOrder = compareIssueStatus(leftIssue, rightIssue);
          if (statusOrder !== 0) {
            return statusOrder;
          }
          return leftIssue.id.localeCompare(rightIssue.id);
        }
        return left.id.localeCompare(right.id);
      }),
    }));
  }, [issue?.links]);

  const operationHistory = issue?.operations ?? [];

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
          <Link
            href="/"
            className="rounded-md border border-border px-3 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            All issues
          </Link>
        }
      />

      <div className="app-scrollarea min-h-0 flex-1 overflow-y-auto">
        <div className="flex w-full flex-col gap-6 px-4 py-6 lg:px-6 xl:px-8">
          {loading && (
            <div className="app-subtle-surface p-5 text-sm text-muted-foreground">
              Loading issue...
            </div>
          )}

          {!loading && issueError && (
            <div className="app-status-warning p-5">
              {issueError.message === "issue not found"
                ? `No issue exists for "${issueID}".`
                : `Issue backend unavailable: ${issueError.message}`}
            </div>
          )}

          {!loading && issue && (
            <motion.div
              key={issueID}
              initial={{ opacity: 0, x: 20 }}
              animate={{ opacity: 1, x: 0 }}
              transition={{ duration: 0.25, ease: "easeOut" }}
              className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_21rem] 2xl:grid-cols-[minmax(0,1.1fr)_24rem]"
            >
              <div className="space-y-6">
                {actionError && <div className="app-status-warning">{actionError}</div>}

                <section className="app-surface rounded-[1.75rem] p-6">
                  <div className="space-y-2">
                    <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                      Canonical summary
                    </p>
                    <h2 className="max-w-3xl text-2xl leading-tight font-semibold tracking-tight text-balance">
                      {formatIssueTitle(issue.raw, 140)}
                    </h2>
                    <p className="text-sm text-muted-foreground">
                      Synthesized from discussion. New posts can update it.
                    </p>
                  </div>

                  <div className="app-subtle-surface mt-6 rounded-[1.5rem] p-5">
                    <DiscussionBody text={issue.raw} />
                  </div>
                </section>

                <section className="app-surface rounded-[1.75rem] p-6">
                  <div className="flex flex-wrap items-center justify-between gap-4">
                    <div>
                      <h3 className="text-lg font-semibold tracking-tight">Related issues</h3>
                      <p className="mt-1 text-sm text-muted-foreground">
                        First-class issue links grouped by relationship type.
                      </p>
                    </div>
                    <span className="app-chip">
                      {issue.links?.length ?? 0} link{(issue.links?.length ?? 0) === 1 ? "" : "s"}
                    </span>
                  </div>

                  {relationshipGroups.length === 0 ? (
                    <div className="app-subtle-surface mt-5 rounded-[1.5rem] p-4 text-sm text-muted-foreground">
                      No issue relationships recorded yet.
                    </div>
                  ) : (
                    <div className="mt-5 space-y-4">
                      {relationshipGroups.map((group) => (
                        <div key={group.type} className="space-y-2">
                          <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                            {group.label}
                          </p>
                          <div className="space-y-2">
                            {group.links.map((link) => (
                              <Link
                                key={link.id}
                                href={
                                  link.relatedIssue
                                    ? `/issues/${link.relatedIssue.id}`
                                    : `/issues/${link.direction === "outgoing" ? link.targetIssueId : link.sourceIssueId}`
                                }
                                className="group app-subtle-surface flex items-start gap-3 rounded-[1.25rem] px-4 py-3 transition-colors hover:bg-accent/70"
                              >
                                <span className="mt-1 size-2 rounded-full bg-emerald-500" />
                                <div className="min-w-0 flex-1">
                                  <div className="flex flex-wrap items-center gap-2">
                                    <p className="truncate text-sm font-medium">
                                      {formatIssueTitle(
                                        link.relatedIssue?.raw ??
                                          `${link.direction === "outgoing" ? link.targetIssueId : link.sourceIssueId}`,
                                        72
                                      )}
                                    </p>
                                    {link.relatedIssue && (
                                      <span
                                        className={cn(
                                          "rounded-full px-1.5 py-0.5 text-[9px] font-medium leading-none",
                                          statusClasses(link.relatedIssue.status)
                                        )}
                                      >
                                        {link.relatedIssue.status === "closed" ? "Closed" : "Open"}
                                      </span>
                                    )}
                                    {link.direction && <span className="app-chip">{link.direction}</span>}
                                  </div>
                                  <p className="mt-1 text-[11px] text-muted-foreground">
                                    {link.relatedIssue?.id ??
                                      (link.direction === "outgoing" ? link.targetIssueId : link.sourceIssueId)}
                                    {" · "}
                                    {formatRelativeTime(link.createdAt)} by {link.createdBy}
                                  </p>
                                  {link.note && (
                                    <p className="mt-2 text-sm text-muted-foreground">
                                      {link.note}
                                    </p>
                                  )}
                                </div>
                              </Link>
                            ))}
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </section>

                <section className="app-surface rounded-[1.75rem] p-6">
                  <div className="flex flex-wrap items-center justify-between gap-4">
                    <h3 className="text-lg font-semibold tracking-tight">Discussion</h3>
                    <span className="app-chip">
                      {discussion.length} post{discussion.length === 1 ? "" : "s"}
                    </span>
                  </div>

                  <div className="mt-5 space-y-4">
                    {discussion.map((post) => {
                      const kind = postKind(post);
                      const refinementIndex =
                        kind === "refinement"
                          ? (refinementIndices.get(post.id) ?? 0)
                          : 0;

                      return (
                        <article
                          key={post.id}
                          className="app-subtle-surface rounded-[1.5rem] p-5"
                        >
                          <div className="flex flex-wrap items-start justify-between gap-3">
                            <div className="space-y-1">
                              <div className="flex flex-wrap items-center gap-2">
                                {kind === "progress" && (
                                  <span className="inline-flex items-center gap-1 rounded-full bg-blue-100 px-2 py-0.5 text-blue-700">
                                    <ArrowUpCircleIcon className="size-3.5" />
                                  </span>
                                )}
                                <span className="text-sm font-semibold">
                                  {kind === "report"
                                    ? "Initial report"
                                    : kind === "progress"
                                      ? "Progress update"
                                      : `Refinement ${refinementIndex}`}
                                </span>
                                {kind === "report" && <span className="app-chip">Starting state</span>}
                              </div>
                              <p className="text-sm text-muted-foreground">
                                {post.createdBy} · {formatRelativeTime(post.createdAt)}
                              </p>
                            </div>
                            <span className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                              {formatDateTime(post.createdAt)}
                            </span>
                          </div>

                          <div className="mt-4">
                            <DiscussionBody text={post.raw} />
                          </div>
                        </article>
                      );
                    })}
                  </div>

                  {issue.status === "closed" ? (
                    <div className="app-subtle-surface mt-5 rounded-[1.5rem] p-4">
                      <p className="text-sm text-muted-foreground">
                        This issue is closed. Reopen it to add refinements or progress updates.
                      </p>
                    </div>
                  ) : (
                    <div className="app-subtle-surface mt-5 rounded-[1.5rem] p-4">
                      <div className="mb-3 flex gap-1">
                        <button
                          type="button"
                          onClick={() => setPostMode("refinement")}
                          className={cn(
                            "rounded-full px-3 py-1 text-xs font-medium transition-colors",
                            postMode === "refinement"
                              ? "bg-amber-100 text-amber-700"
                              : "bg-muted text-muted-foreground hover:bg-muted/80"
                          )}
                        >
                          Refinement
                        </button>
                        <button
                          type="button"
                          onClick={() => setPostMode("progress")}
                          className={cn(
                            "rounded-full px-3 py-1 text-xs font-medium transition-colors",
                            postMode === "progress"
                              ? "bg-blue-100 text-blue-700"
                              : "bg-muted text-muted-foreground hover:bg-muted/80"
                          )}
                        >
                          Progress
                        </button>
                      </div>
                      <div className="flex items-start gap-3">
                        <div
                          className={cn(
                            "mt-0.5 rounded-full p-2",
                            postMode === "progress"
                              ? "bg-blue-100 text-blue-700"
                              : "bg-amber-100 text-amber-700"
                          )}
                        >
                          {postMode === "progress" ? (
                            <ArrowUpCircleIcon className="size-4" />
                          ) : (
                            <SparklesIcon className="size-4" />
                          )}
                        </div>
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-medium">
                            {postMode === "progress"
                              ? "Post a progress update"
                              : "Refine this issue"}
                          </p>
                          <p className="mt-1 text-sm text-muted-foreground">
                            {postMode === "progress"
                              ? "Report work done toward resolving this issue."
                              : "Add context, corrections, or feedback."}
                          </p>
                          <textarea
                            value={refineInput}
                            onChange={(event) => {
                              setRefineInput(event.target.value);
                              if (actionError) {
                                setActionError(null);
                              }
                            }}
                            onKeyDown={(event) => {
                              if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
                                event.preventDefault();
                                void handleSubmit();
                              }
                            }}
                            placeholder={
                              postMode === "progress"
                                ? "Report work done toward resolving this issue..."
                                : "Add more context, corrections, or feedback..."
                            }
                            disabled={refinePending || progressPending}
                            className="mt-4 min-h-[120px] w-full resize-y rounded-xl border border-input/80 bg-background/70 px-3 py-2 text-sm leading-relaxed placeholder:text-muted-foreground/50 focus:outline-none focus:ring-2 focus:ring-ring/40"
                          />
                          <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
                            <p className="text-[11px] text-muted-foreground">
                              Press <span className="font-medium text-foreground">Cmd/Ctrl + Enter</span>{" "}
                              to post.
                            </p>
                            <Button
                              type="button"
                              onClick={() => void handleSubmit()}
                              disabled={refinePending || progressPending || refineInput.trim().length === 0}
                            >
                              {refinePending || progressPending
                                ? "Posting..."
                                : postMode === "progress"
                                  ? "Post progress"
                                  : "Post refinement"}
                            </Button>
                          </div>
                        </div>
                      </div>
                    </div>
                  )}
                </section>
              </div>

              <aside className="space-y-4 xl:sticky xl:top-6 xl:self-start">
                {!mapError && currentMapIssue && (
                  <section className="app-surface rounded-[1.75rem] p-6">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                          Related context
                        </p>
                        <h3 className="mt-1 text-base font-semibold tracking-tight">
                          Map context and nearby issues
                        </h3>
                        <p className="mt-1 text-sm text-muted-foreground">
                          Open issues are surfaced ahead of closed ones.
                        </p>
                      </div>
                      <div className="flex flex-wrap gap-1.5">
                        <span className="app-chip">
                          {currentClusters.length > 0
                            ? `${currentClusters.length} cluster${currentClusters.length === 1 ? "" : "s"}`
                            : "No cluster"}
                        </span>
                        {semanticNeighbors.length > 0 && (
                          <span className="app-chip">
                            {semanticNeighbors.length} semantic
                          </span>
                        )}
                        {clusterNeighbors.length > 0 && (
                          <span className="app-chip">
                            {clusterNeighbors.length} nearby
                          </span>
                        )}
                      </div>
                    </div>

                    <div className="app-subtle-surface mt-4 rounded-[1.25rem] p-3">
                      <div className="relative aspect-[16/11] w-full overflow-hidden rounded-[1rem]">
                        <IssueMapCanvas
                          width={320}
                          height={220}
                          viewBox="0 0 320 220"
                          preserveAspectRatio="xMidYMid meet"
                          className="absolute inset-0 h-full w-full"
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
                      </div>

                      <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-muted-foreground">
                        <span className="app-chip inline-flex items-center gap-1">
                          <span className="size-2 rounded-full bg-slate-900" />
                          Current
                        </span>
                        <span className="app-chip inline-flex items-center gap-1">
                          <span className="size-2 rounded-full bg-blue-600" />
                          Cluster
                        </span>
                        <span className="app-chip inline-flex items-center gap-1">
                          <span className="size-2 rounded-full bg-amber-500" />
                          Semantic
                        </span>
                      </div>
                    </div>

                    {semanticNeighbors.length > 0 && (
                      <div className="mt-4">
                        <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                          Semantic neighbors
                        </p>
                        <div className="mt-2 space-y-1.5">
                          {semanticNeighbors.map(({ issue: neighbor, similarity }) => (
                            <Link
                              key={neighbor.id}
                              href={`/issues/${neighbor.id}`}
                              className="group app-subtle-surface flex items-center gap-2 px-2.5 py-1.5 transition-colors hover:bg-accent/70"
                            >
                              <span className="size-2 rounded-full bg-amber-500" />
                              <p className="min-w-0 flex-1 truncate text-xs">
                                {formatIssueTitle(neighbor.raw, 44)}
                              </p>
                              <span
                                className={cn(
                                  "shrink-0 rounded-full px-1.5 py-0.5 text-[9px] font-medium leading-none",
                                  statusClasses(neighbor.status)
                                )}
                              >
                                {neighbor.status === "closed" ? "Closed" : "Open"}
                              </span>
                              <span className="text-[10px] tabular-nums text-muted-foreground">
                                {(similarity * 100).toFixed(0)}%
                              </span>
                            </Link>
                          ))}
                        </div>
                      </div>
                    )}

                    {clusterNeighbors.length > 0 && (
                      <div className="mt-4">
                        <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                          Clustered nearby
                        </p>
                        <div className="mt-2 space-y-1.5">
                          {clusterNeighbors.map(({ issue: neighbor }) => (
                            <Link
                              key={neighbor.id}
                              href={`/issues/${neighbor.id}`}
                              className="group app-subtle-surface flex items-center gap-2 px-2.5 py-1.5 transition-colors hover:bg-accent/70"
                            >
                              <span className="size-2 rounded-full bg-blue-600" />
                              <p className="min-w-0 flex-1 truncate text-xs">
                                {formatIssueTitle(neighbor.raw, 44)}
                              </p>
                              <span
                                className={cn(
                                  "shrink-0 rounded-full px-1.5 py-0.5 text-[9px] font-medium leading-none",
                                  statusClasses(neighbor.status)
                                )}
                              >
                                {neighbor.status === "closed" ? "Closed" : "Open"}
                              </span>
                            </Link>
                          ))}
                        </div>
                      </div>
                    )}
                  </section>
                )}

                <section className="app-surface rounded-[1.5rem] p-5">
                  <div className="flex items-center gap-2">
                    <HashIcon className="size-4 text-muted-foreground" />
                    <h3 className="text-sm font-semibold">Issue details</h3>
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
                        Assigned to
                      </dt>
                      <dd>
                        {assignEditing ? (
                          <div className="flex items-center gap-1.5">
                            <input
                              type="text"
                              value={assignInput}
                              onChange={(e) => setAssignInput(e.target.value)}
                              onKeyDown={(e) => {
                                if (e.key === "Enter") {
                                  e.preventDefault();
                                  void handleAssign(assignInput);
                                }
                                if (e.key === "Escape") {
                                  setAssignEditing(false);
                                }
                              }}
                              autoFocus
                              placeholder="Name..."
                              disabled={assignPending}
                              className="min-w-0 flex-1 rounded-lg border border-input/80 bg-background/70 px-2 py-1 text-sm placeholder:text-muted-foreground/50 focus:outline-none focus:ring-2 focus:ring-ring/40"
                            />
                            <Button
                              type="button"
                              size="xs"
                              onClick={() => void handleAssign(assignInput)}
                              disabled={assignPending}
                            >
                              {assignPending ? "..." : "Save"}
                            </Button>
                            <Button
                              type="button"
                              size="xs"
                              variant="ghost"
                              onClick={() => setAssignEditing(false)}
                              disabled={assignPending}
                            >
                              Cancel
                            </Button>
                          </div>
                        ) : (
                          <div className="flex items-center gap-1.5">
                            <button
                              type="button"
                              onClick={() => {
                                setAssignInput(issue.assignedTo ?? "");
                                setAssignEditing(true);
                              }}
                              className="group flex items-center gap-1.5 rounded-lg px-1 py-0.5 text-sm transition-colors hover:bg-accent/70"
                            >
                              <UserIcon className="size-3.5 text-muted-foreground" />
                              {issue.assignedTo ? (
                                <span className="font-medium text-violet-700">{issue.assignedTo}</span>
                              ) : (
                                <span className="text-muted-foreground">Unassigned</span>
                              )}
                            </button>
                            {user && issue.assignedTo !== user.displayName && (
                              <Button
                                type="button"
                                size="xs"
                                variant="ghost"
                                onClick={() => void handleAssign(user.displayName)}
                                disabled={assignPending}
                              >
                                {assignPending ? "..." : "Assign to me"}
                              </Button>
                            )}
                          </div>
                        )}
                      </dd>
                    </div>

                    <div className="space-y-1">
                      <dt className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                        Tags
                      </dt>
                      <dd className="flex flex-wrap gap-1.5">
                        {issue.tags.length > 0 ? (
                          issue.tags.map((tag) => (
                            <Link
                              key={tag}
                              href={tagHref(tag)}
                              className="rounded-full border px-2.5 py-1 text-[11px] font-medium transition-colors hover:bg-accent/70"
                              style={entityStyle(tag)}
                            >
                              {tag}
                            </Link>
                          ))
                        ) : (
                          <span className="text-sm text-muted-foreground">None</span>
                        )}
                      </dd>
                    </div>

                    {issue.tagScores && issue.tagScores.length > 0 && (
                      <TagRelevanceBars tags={issue.tagScores} />
                    )}

                    <div className="space-y-1">
                      <dt className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                        Discussion
                      </dt>
                      <dd className="text-sm">
                        <div className="flex items-center gap-2">
                          <MessageSquareMoreIcon className="size-4 text-muted-foreground" />
                          <span>
                            {discussion.length} post{discussion.length === 1 ? "" : "s"}
                          </span>
                        </div>
                        {(() => {
                          const refinementCount = discussion.filter((p) => postKind(p) === "refinement").length;
                          const progressCount = discussion.filter((p) => postKind(p) === "progress").length;
                          if (progressCount > 0) {
                            return (
                              <p className="mt-1 text-[11px] text-muted-foreground">
                                {refinementCount} refinement{refinementCount === 1 ? "" : "s"}, {progressCount} progress
                              </p>
                            );
                          }
                          return null;
                        })()}
                      </dd>
                    </div>

                    {timeline.map((entry) => (
                      <div key={entry.label} className="space-y-1">
                        <dt className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                          {entry.label}
                        </dt>
                        <dd>
                          <p className="text-sm">{entry.value}</p>
                          <p className="mt-0.5 text-[11px] text-muted-foreground">
                            {entry.meta}
                          </p>
                        </dd>
                      </div>
                    ))}
                  </dl>
                </section>

                <section className="app-surface rounded-[1.5rem] p-5">
                  <div className="flex items-center gap-2">
                    <SparklesIcon className="size-4 text-muted-foreground" />
                    <h3 className="text-sm font-semibold">Operation history</h3>
                  </div>

                  {operationHistory.length === 0 ? (
                    <p className="mt-4 text-sm text-muted-foreground">
                      No grouped split, combine, or link operations yet.
                    </p>
                  ) : (
                    <div className="mt-4 space-y-3">
                      {operationHistory.map((operation) => (
                        <article key={operation.id} className="app-subtle-surface rounded-[1.25rem] p-4">
                          <div className="flex flex-wrap items-center justify-between gap-2">
                            <div>
                              <p className="text-sm font-semibold">{formatOperationKind(operation.kind)}</p>
                              <p className="text-[11px] text-muted-foreground">
                                {formatRelativeTime(operation.createdAt)} by {operation.createdBy}
                              </p>
                            </div>
                            <span className="app-chip">{operation.id}</span>
                          </div>
                          {operation.participants && operation.participants.length > 0 && (
                            <div className="mt-3 flex flex-wrap gap-1.5">
                              {operation.participants.map((participant) => (
                                <Link
                                  key={`${operation.id}-${participant.issueId}-${participant.role}`}
                                  href={`/issues/${participant.issueId}`}
                                  className="rounded-full border border-border/70 px-2.5 py-1 text-[11px] font-medium transition-colors hover:bg-accent/70"
                                >
                                  {(participant.issue?.id ?? participant.issueId)} · {participant.role}
                                </Link>
                              ))}
                            </div>
                          )}
                          {operation.note && (
                            <p className="mt-3 text-sm text-muted-foreground">{operation.note}</p>
                          )}
                        </article>
                      ))}
                    </div>
                  )}
                </section>

                <section className="app-surface rounded-[1.5rem] p-5">
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
                      {copyState === "text-copied" ? "Copied summary" : "Copy summary"}
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
                    copies the current summary.
                  </p>
                </section>

                {mapError && (
                  <div className="app-status-warning text-sm">
                    Map context unavailable: {mapError.message}
                  </div>
                )}
              </aside>
            </motion.div>
          )}
        </div>
      </div>
    </AppShell>
  );
}
