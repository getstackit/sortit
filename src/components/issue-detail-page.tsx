"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
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
  type IssueMapCanvasLine,
  type IssueMapCanvasNode,
} from "@/components/issue-map-canvas";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
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
import { rememberRecentIssue } from "@/hooks/use-recent-history";
import { cn } from "@/lib/utils";
import { formatRelativeTime } from "@/lib/format";
import { Badge } from "@/components/ui/badge";
import { CloseIssueModal } from "@/components/close-issue-modal";
import { Markdown } from "@/components/markdown";

import { TagRelevanceBars } from "@/components/tag-relevance-bars";

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
const MINI_MAP_VIEWBOX_WIDTH = 640;
const MINI_MAP_VIEWBOX_HEIGHT = 360;
const MINI_MAP_CENTER_X = MINI_MAP_VIEWBOX_WIDTH / 2;
const MINI_MAP_CENTER_Y = MINI_MAP_VIEWBOX_HEIGHT / 2;
const MINI_MAP_INNER_RADIUS = Math.min(MINI_MAP_VIEWBOX_WIDTH, MINI_MAP_VIEWBOX_HEIGHT) / 2 - 44;

function formatDateTime(value: string) {
  return new Date(value).toLocaleString();
}

function formatIssueTitle(raw: string, maxLength = 84) {
  const normalized = raw.replace(/\s+/g, " ").trim();
  return normalized.length > maxLength
    ? `${normalized.slice(0, maxLength).trimEnd()}...`
    : normalized;
}

function formatPercent(value: number) {
  return `${Math.round(value * 100)}%`;
}

function statusClasses(status: IssueRecord["status"]) {
  return status === "closed"
    ? "bg-slate-200 text-slate-700"
    : "bg-emerald-100 text-emerald-700";
}

function enrichmentClasses(status: IssueRecord["enrichmentStatus"]) {
  return status === "failed"
    ? "bg-rose-100 text-rose-700"
    : "bg-amber-100 text-amber-700";
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

function formatPostKindLabel(kind: IssuePostKind, refinementIndex: number) {
  switch (kind) {
    case "report":
      return "Initial report";
    case "progress":
      return "Progress update";
    case "closed":
      return "Closed";
    case "reopened":
      return "Reopened";
    case "assigned":
      return "Assignment";
    case "refinement":
    default:
      return `Refinement ${refinementIndex}`;
  }
}

function formatLinkType(
  type: IssueLinkRecord["type"],
  direction?: IssueLinkRecord["direction"]
) {
  switch (type) {
    case "parent_of":
      return direction === "incoming" ? "Child of" : "Parent of";
    case "child_of":
      return direction === "incoming" ? "Parent of" : "Child of";
    case "merged_into":
      return direction === "incoming" ? "Merged from" : "Merged into";
    case "derived_from":
      return direction === "incoming" ? "Source for" : "Derived from";
    case "duplicate_of":
      return direction === "incoming" ? "Has duplicate" : "Duplicate of";
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
  return cluster.issueIds.includes(issue.id);
}

function projectIssueNeighborhood(
  dx: number,
  dy: number,
  scale: number
) {
  return {
    x: MINI_MAP_CENTER_X + dx * scale,
    y: MINI_MAP_CENTER_Y - dy * scale,
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
  return <Markdown>{text}</Markdown>;
}

export function IssueDetailPage({ issueID }: { issueID: string }) {
  const params = useParams<{ id?: string }>();
  const resolvedIssueID = useMemo(() => {
    const candidates = [issueID, params?.id];
    for (const candidate of candidates) {
      const normalized = typeof candidate === "string" ? candidate.trim() : "";
      if (normalized && normalized !== "undefined" && normalized !== "null") {
        return normalized;
      }
    }
    return "";
  }, [issueID, params]);
  const { data: issue, error: issueError, isLoading: loading, mutate: mutateIssue } = useIssue(resolvedIssueID);
  const { data: mapData, error: mapError, mutate: mutateMap } = useIssueMapData();
  const [actionError, setActionError] = useState<string | null>(null);
  const [copyState, setCopyState] = useState<
    "idle" | "text-copied" | "link-copied" | "error"
  >("idle");
  const [idCopied, setIdCopied] = useState(false);
  const [statusPending, setStatusPending] = useState(false);
  const [closeModalOpen, setCloseModalOpen] = useState(false);
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

  useEffect(() => {
    if (!issue) {
      return;
    }

    rememberRecentIssue(issue);
  }, [issue]);

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

    if (issue.status === "open") {
      setCloseModalOpen(true);
      return;
    }

    setStatusPending(true);

    try {
      const updated = await reopenIssue(issue.id);
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

  const handleCloseConfirm = useCallback(async (reason: string, reasonNote: string) => {
    if (!issue) return;
    setStatusPending(true);
    try {
      const updated = await closeIssue(issue.id, { reason, reasonNote });
      mutateIssue(updated, { revalidate: false });
      setActionError(null);
      setCloseModalOpen(false);
    } catch (caughtError) {
      const message =
        caughtError instanceof Error
          ? caughtError.message
          : "Unknown backend error";
      setActionError(message);
    } finally {
      setStatusPending(false);
    }
  }, [issue, mutateIssue]);

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

  const clusterPeers = useMemo<ClusterNeighbor[]>(() => {
    return clusterNeighbors.filter((neighbor) => !semanticNeighborIds.has(neighbor.issue.id));
  }, [clusterNeighbors, semanticNeighborIds]);

  const neighborhoodIssues = useMemo(() => {
    const entries = new Map<string, MapIssue>();
    if (currentMapIssue) {
      entries.set(currentMapIssue.id, currentMapIssue);
    }

    for (const { issue: neighbor } of semanticNeighbors) {
      entries.set(neighbor.id, neighbor);
    }

    for (const { issue: neighbor } of clusterPeers) {
      entries.set(neighbor.id, neighbor);
    }

    return Array.from(entries.values());
  }, [currentMapIssue, semanticNeighbors, clusterPeers]);

  const neighborhoodScale = useMemo(() => {
    if (!currentMapIssue) {
      return 1;
    }

    let maxOffset = 0.1;

    for (const entry of neighborhoodIssues) {
      maxOffset = Math.max(
        maxOffset,
        Math.abs(entry.x - currentMapIssue.x),
        Math.abs(entry.y - currentMapIssue.y)
      );
    }

    return MINI_MAP_INNER_RADIUS / maxOffset;
  }, [currentMapIssue, neighborhoodIssues]);

  const miniMapEdges = useMemo<IssueMapCanvasLine[]>(() => {
    if (!currentMapIssue) {
      return [];
    }

    return semanticNeighbors.map(({ issue: neighbor, similarity }) => {
      const from = projectIssueNeighborhood(0, 0, neighborhoodScale);
      const to = projectIssueNeighborhood(
        neighbor.x - currentMapIssue.x,
        neighbor.y - currentMapIssue.y,
        neighborhoodScale
      );

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
  }, [currentMapIssue, neighborhoodScale, semanticNeighbors]);

  const miniMapNodes = useMemo<IssueMapCanvasNode[]>(() => {
    if (!currentMapIssue) {
      return [];
    }

    return neighborhoodIssues.map((entry) => {
      const point = projectIssueNeighborhood(
        entry.x - currentMapIssue.x,
        entry.y - currentMapIssue.y,
        neighborhoodScale
      );
      const isCurrent = entry.id === currentMapIssue?.id;
      const isSemantic = semanticNeighborIds.has(entry.id);
      const isClusterPeer = !isCurrent && !isSemantic;
      const label =
        isCurrent || isSemantic
          ? formatIssueTitle(entry.raw, isCurrent ? 32 : 24)
          : undefined;

      return {
        key: entry.id,
        cx: point.x,
        cy: point.y,
        radius: isCurrent ? 7 : isClusterPeer ? 3.5 : 4.5,
        fill: isCurrent
          ? "#0f172a"
          : isClusterPeer
            ? "#2563eb"
            : "#f59e0b",
        fillOpacity: isCurrent ? 1 : isClusterPeer ? 0.7 : 0.95,
        stroke: isCurrent ? "#ffffff" : undefined,
        strokeWidth: isCurrent ? 2 : 0,
        label,
        labelY: point.y - (isCurrent ? 18 : 14),
        labelClassName: "text-[10px] font-semibold tracking-[0.01em]",
        labelFillOpacity: isCurrent ? 1 : 0.72,
      };
    });
  }, [
    currentMapIssue,
    neighborhoodIssues,
    neighborhoodScale,
    semanticNeighborIds,
  ]);

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

    if (typeof issue.lifecycleMetrics?.stability === "number" && typeof issue.lifecycleMetrics?.churn === "number") {
      const transitions = issue.lifecycleMetrics.transitionCount ?? 0;
      const snapshots = issue.lifecycleMetrics.snapshotCount ?? transitions + 1;
      entries.push({
        label: "Stability",
        value: formatPercent(issue.lifecycleMetrics.stability),
        meta: `${formatPercent(issue.lifecycleMetrics.churn)} churn across ${transitions} transition${transitions === 1 ? "" : "s"} from ${snapshots} snapshot${snapshots === 1 ? "" : "s"}`,
      });
    }

    if (typeof issue.lifecycleMetrics?.velocity === "number") {
      const recentActivityCount = issue.lifecycleMetrics.recentActivityCount ?? 0;
      entries.push({
        label: "Velocity",
        value: formatPercent(issue.lifecycleMetrics.velocity),
        meta: `${recentActivityCount} recent activity event${recentActivityCount === 1 ? "" : "s"} in the last 30 days`,
      });
    }

    return entries;
  }, [issue]);

  const relationshipGroups = useMemo(() => {
    const groups = new Map<string, IssueLinkRecord[]>();
    for (const link of issue?.links ?? []) {
      const key = `${link.type}:${link.direction ?? "unknown"}`;
      const current = groups.get(key) ?? [];
      current.push(link);
      groups.set(key, current);
    }

    return [...groups.entries()].map(([key, links]) => ({
      type: key,
      label: formatLinkType(links[0].type, links[0].direction),
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
        title={issue ? formatIssueTitle(issue.raw) : resolvedIssueID || "Issue"}
        eyebrow={
          <Breadcrumb>
            <BreadcrumbList>
              <BreadcrumbItem>
                <BreadcrumbLink render={<Link href="/" />}>All issues</BreadcrumbLink>
              </BreadcrumbItem>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbPage className="inline-flex items-center gap-1">
                  {issue?.id ?? resolvedIssueID ?? "Issue"}
                  {(issue?.id ?? resolvedIssueID) && (
                    <button
                      type="button"
                      className="inline-flex items-center justify-center rounded-sm p-0.5 text-muted-foreground hover:text-foreground transition-colors"
                      aria-label="Copy issue ID"
                      onClick={() => {
                        const id = issue?.id ?? resolvedIssueID;
                        if (id) void copyText(id).then(() => {
                          setIdCopied(true);
                          setTimeout(() => setIdCopied(false), 1500);
                        });
                      }}
                    >
                      {idCopied ? (
                        <CheckIcon className="size-3" aria-hidden="true" />
                      ) : (
                        <CopyIcon className="size-3" aria-hidden="true" />
                      )}
                    </button>
                  )}
                </BreadcrumbPage>
              </BreadcrumbItem>
            </BreadcrumbList>
          </Breadcrumb>
        }
        subtitle={issue ? `${issue.createdBy} · ${formatRelativeTime(issue.createdAt)}` : undefined}
        meta={
          issue ? (
            <>
              <span
                className={cn(
                  "rounded-full px-2 py-0.5 text-[11px] font-medium",
                  statusClasses(issue.status)
                )}
              >
                {issue.status === "closed" ? "Closed" : "Open"}
              </span>
              {issue.enrichmentStatus && issue.enrichmentStatus !== "complete" && (
                <>
                  <span className="mx-1 text-border">·</span>
                  <span
                    className={cn(
                      "rounded-full px-2 py-0.5 text-[11px] font-medium",
                      enrichmentClasses(issue.enrichmentStatus)
                    )}
                  >
                    {issue.enrichmentStatus === "failed" ? "Analysis failed" : "Analyzing"}
                  </span>
                </>
              )}
            </>
          ) : null
        }
      />

      <div className="app-scrollarea min-h-0 flex-1 overflow-y-auto">
        <div className="flex w-full flex-col gap-6 px-4 py-6 lg:px-6 xl:px-8">
          {loading && (
            <div className="app-subtle-surface p-5 text-sm text-muted-foreground">
              Loading issue...
            </div>
          )}

          {!loading && !resolvedIssueID && (
            <div className="app-status-warning p-5">No issue ID was provided in the route.</div>
          )}

          {!loading && resolvedIssueID && issueError && (
            <div className="app-status-warning p-5">
              {issueError.message === "issue not found"
                ? `No issue exists for "${resolvedIssueID}".`
                : `Issue backend unavailable: ${issueError.message}`}
            </div>
          )}

          {!loading && issue && (
            <motion.div
              key={resolvedIssueID}
              initial={{ opacity: 0, x: 20 }}
              animate={{ opacity: 1, x: 0 }}
              transition={{ duration: 0.25, ease: "easeOut" }}
              className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_21rem] 2xl:grid-cols-[minmax(0,1.1fr)_24rem]"
            >
              <div className="space-y-6">
                {actionError && <div className="app-status-warning">{actionError}</div>}
                {issue.enrichmentStatus === "pending" && (
                  <div className="app-status-warning">
                    Background analysis is running. Tags, embeddings, and the canonical summary may still update.
                  </div>
                )}
                {issue.enrichmentStatus === "failed" && (
                  <div className="app-status-warning">
                    Background analysis failed{issue.enrichmentError ? `: ${issue.enrichmentError}` : "."}
                  </div>
                )}

                <section className="app-surface rounded-[1.75rem] p-6">
                  <div>
                    <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                      Canonical summary
                    </p>
                    <h2 className="sr-only">{issue.raw}</h2>
                  </div>
                  <div className="mt-4">
                    <DiscussionBody text={issue.raw} />
                  </div>
                </section>

                {relationshipGroups.length > 0 && (
                <section className="app-surface rounded-[1.75rem] p-6">
                  <div className="flex flex-wrap items-center justify-between gap-4">
                    <div>
                      <h3 className="text-lg font-semibold tracking-tight">Related issues</h3>
                      <p className="mt-1 text-sm text-muted-foreground">
                        First-class issue links grouped by relationship type.
                      </p>
                    </div>
                    <Badge variant="outline">
                      {issue.links?.length ?? 0} link{(issue.links?.length ?? 0) === 1 ? "" : "s"}
                    </Badge>
                  </div>

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
                                  {link.direction && <Badge variant="outline">{link.direction}</Badge>}
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
                </section>
                )}

                {!mapError && currentMapIssue && (
                  <section className="app-surface rounded-[1.75rem] p-6">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                          Related context
                        </p>
                        <h3 className="mt-1 text-lg font-semibold tracking-tight">
                          Issue neighborhood
                        </h3>
                        <p className="mt-1 text-sm text-muted-foreground">
                          Centered on this issue. The graphic focuses on strongest semantic matches, with the current issue fixed at the middle.
                        </p>
                      </div>
                      <div className="flex flex-wrap gap-1.5">
                        {semanticNeighbors.length > 0 && (
                          <Badge variant="outline">
                            {semanticNeighbors.length} semantic
                          </Badge>
                        )}
                      </div>
                    </div>

                    <div className="app-subtle-surface mt-4 rounded-[1.25rem] p-3">
                      <div className="relative aspect-[16/9] w-full overflow-hidden rounded-[1rem]">
                        <IssueMapCanvas
                          width="100%"
                          height="100%"
                          viewBox={`0 0 ${MINI_MAP_VIEWBOX_WIDTH} ${MINI_MAP_VIEWBOX_HEIGHT}`}
                          preserveAspectRatio="xMidYMid meet"
                          className="absolute inset-0 h-full w-full"
                          role="img"
                          aria-label={`Issue neighborhood for ${currentMapIssue.id}`}
                          background={
                            <>
                              <rect
                                x="0"
                                y="0"
                                width={MINI_MAP_VIEWBOX_WIDTH}
                                height={MINI_MAP_VIEWBOX_HEIGHT}
                                rx="18"
                                fill="currentColor"
                                className="text-background"
                              />
                              <circle
                                cx={MINI_MAP_CENTER_X}
                                cy={MINI_MAP_CENTER_Y}
                                r={58}
                                fill="none"
                                stroke="currentColor"
                                strokeOpacity="0.08"
                              />
                              <circle
                                cx={MINI_MAP_CENTER_X}
                                cy={MINI_MAP_CENTER_Y}
                                r={112}
                                fill="none"
                                stroke="currentColor"
                                strokeOpacity="0.05"
                                strokeDasharray="5 7"
                              />
                            </>
                          }
                          edges={miniMapEdges}
                          nodes={miniMapNodes}
                        />
                      </div>

                      <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-muted-foreground">
                        <Badge variant="outline" className="inline-flex items-center gap-1">
                          <span className="size-2 rounded-full bg-slate-900" />
                          Current issue
                        </Badge>
                        <Badge variant="outline" className="inline-flex items-center gap-1">
                          <span className="size-2 rounded-full bg-amber-500" />
                          Semantic
                        </Badge>
                        <Badge variant="outline" className="inline-flex items-center gap-1">
                          <span className="size-2 rounded-full bg-blue-600" />
                          Cluster
                        </Badge>
                      </div>
                    </div>

                    <div className="mt-5 grid gap-5 sm:grid-cols-2">
                      {semanticNeighbors.length > 0 && (
                        <div>
                          <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                            Semantic neighbors
                          </p>
                          <div className="mt-2 space-y-1.5">
                            {semanticNeighbors.map(({ issue: neighbor, similarity }) => (
                              <Link
                                key={neighbor.id}
                                href={`/issues/${neighbor.id}`}
                                className="group app-subtle-surface flex items-center gap-2 rounded-lg px-2.5 py-1.5 transition-colors hover:bg-accent/70"
                              >
                                <span className="size-2 rounded-full bg-amber-500" />
                                <p className="min-w-0 flex-1 truncate text-xs">
                                  {formatIssueTitle(neighbor.raw, 52)}
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

                      {clusterPeers.length > 0 && (
                        <div>
                          <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                            Same cluster
                          </p>
                          <div className="mt-2 space-y-1.5">
                            {clusterPeers.map(({ issue: neighbor }) => (
                              <Link
                                key={neighbor.id}
                                href={`/issues/${neighbor.id}`}
                                className="group app-subtle-surface flex items-center gap-2 rounded-lg px-2.5 py-1.5 transition-colors hover:bg-accent/70"
                              >
                                <span className="size-2 rounded-full bg-blue-600" />
                                <p className="min-w-0 flex-1 truncate text-xs">
                                  {formatIssueTitle(neighbor.raw, 52)}
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
                    </div>
                  </section>
                )}

                <section className="app-surface rounded-[1.75rem] p-6">
                  <div className="flex flex-wrap items-center justify-between gap-4">
                    <h3 className="text-lg font-semibold tracking-tight">Discussion</h3>
                    <Badge variant="outline">
                      {discussion.length} post{discussion.length === 1 ? "" : "s"}
                    </Badge>
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
                                  {formatPostKindLabel(kind, refinementIndex)}
                                </span>
                                {kind === "report" && <Badge variant="outline">Starting state</Badge>}
                                {kind === "closed" && <Badge variant="outline">Status</Badge>}
                                {kind === "reopened" && <Badge variant="outline">Status</Badge>}
                                {kind === "assigned" && <Badge variant="outline">Routing</Badge>}
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
                              Markdown supported. Press <span className="font-medium text-foreground">Cmd/Ctrl + Enter</span>{" "}
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
                <section className="app-surface rounded-[1.5rem] p-5">
                  <div className="flex items-center gap-2">
                    <HashIcon className="size-4 text-muted-foreground" />
                    <h3 className="text-sm font-semibold">Issue details</h3>
                  </div>

                  <dl className="mt-4 space-y-4 text-sm">
                    <div className="space-y-1">
                      <dt className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                        Assigned to
                      </dt>
                      <dd>
                        {assignEditing ? (
                          <div className="flex flex-col gap-2">
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
                              className="w-full rounded-md border border-input/80 bg-background/70 px-2.5 py-1.5 text-sm placeholder:text-muted-foreground/50 focus:outline-none focus:ring-2 focus:ring-ring/40"
                            />
                            <div className="flex items-center gap-1.5">
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
                          </div>
                        ) : (
                          <div className="flex items-center gap-2">
                            <button
                              type="button"
                              onClick={() => {
                                setAssignInput(issue.assignedTo ?? "");
                                setAssignEditing(true);
                              }}
                              className="inline-flex items-center gap-1.5 rounded-md px-1.5 py-0.5 text-sm transition-colors hover:bg-accent/70"
                            >
                              <UserIcon className="size-3 text-muted-foreground" />
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
                                className="text-[11px]"
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

                {issue.tagScores && issue.tagScores.length > 0 && (
                  <section className="app-surface rounded-[1.5rem] p-5">
                    <h3 className="text-sm font-semibold">Classification confidence</h3>
                    <div className="mt-3">
                      <TagRelevanceBars tags={issue.tagScores} />
                    </div>
                  </section>
                )}

                <section className="app-surface rounded-[1.5rem] p-5">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <h3 className="text-sm font-semibold">Operation history</h3>
                      <p className="mt-0.5 text-[11px] text-muted-foreground">
                        Split, combine, and link operations involving this issue.
                      </p>
                    </div>
                    <Badge variant="outline">
                      {operationHistory.length} op{operationHistory.length === 1 ? "" : "s"}
                    </Badge>
                  </div>

                  {operationHistory.length === 0 ? (
                    <div className="app-subtle-surface mt-4 rounded-[1.25rem] p-4 text-sm text-muted-foreground">
                      No split, combine, or link operations yet.
                    </div>
                  ) : (
                    <div className="mt-4 space-y-4">
                      {operationHistory.map((operation) => (
                        <div key={operation.id} className="space-y-2">
                          <div className="flex items-center gap-2">
                            <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                              {formatOperationKind(operation.kind)}
                            </p>
                            <span className="text-[11px] text-muted-foreground">
                              · {formatRelativeTime(operation.createdAt)} by {operation.createdBy}
                            </span>
                          </div>
                          {operation.note && (
                            <p className="text-sm text-muted-foreground">{operation.note}</p>
                          )}
                          {operation.participants && operation.participants.length > 0 && (
                            <div className="space-y-2">
                              {operation.participants
                                .filter((p) => p.issueId !== issue.id)
                                .map((participant) => (
                                <Link
                                  key={`${operation.id}-${participant.issueId}-${participant.role}`}
                                  href={`/issues/${participant.issueId}`}
                                  className="app-subtle-surface flex items-center gap-2 rounded-full px-3 py-1.5 text-[11px] font-medium transition-colors hover:bg-accent/70"
                                >
                                  {participant.issue?.id ?? participant.issueId}
                                  {participant.issue && (
                                    <span
                                      className={cn(
                                        "rounded-full px-1.5 py-0.5 text-[9px] font-medium leading-none",
                                        statusClasses(participant.issue.status)
                                      )}
                                    >
                                      {participant.issue.status === "closed" ? "Closed" : "Open"}
                                    </span>
                                  )}
                                  <span className="text-muted-foreground">{participant.role}</span>
                                </Link>
                              ))}
                            </div>
                          )}
                        </div>
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
      <CloseIssueModal
        open={closeModalOpen}
        onOpenChange={setCloseModalOpen}
        onConfirm={handleCloseConfirm}
      />
    </AppShell>
  );
}
