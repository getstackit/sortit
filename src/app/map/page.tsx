"use client";

import { useState, useRef, useCallback, useEffect, useMemo } from "react";
import { AppSidebar } from "@/components/app-sidebar";
import {
  SidebarInset,
  SidebarProvider,
} from "@/components/ui/sidebar";

type TagRelevance = { tag: string; relevance: number };

type MapIssue = {
  id: string;
  raw: string;
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

type EdgeData = {
  edges: MapEdge[];
};

type Viewport = {
  xMin: number;
  xMax: number;
  yMin: number;
  yMax: number;
};

const TAG_COLORS: Record<string, string> = {
  bug: "#ef4444",
  crash: "#dc2626",
  feature: "#a855f7",
  idea: "#a855f7",
  improvement: "#22c55e",
  ui: "#3b82f6",
  ux: "#3b82f6",
  frontend: "#60a5fa",
  performance: "#f59e0b",
  safari: "#f59e0b",
  onboarding: "#06b6d4",
  search: "#8b5cf6",
  export: "#ec4899",
};

const PADDING = 60;
const DEFAULT_VIEWPORT: Viewport = {
  xMin: 0,
  xMax: 1,
  yMin: 0,
  yMax: 1,
};
const EMPTY_ISSUES: MapIssue[] = [];
const EMPTY_EDGES: MapEdge[] = [];
const EMPTY_CLUSTERS: MapCluster[] = [];
const MIN_VIEW_SIZE = 0.12;
const MAX_VIEW_SIZE = 2.4;
const PAN_OVERSCAN = 0.35;
const EDGE_FETCH_DEBOUNCE_MS = 120;
const MAX_AMBIENT_VIEWPORT_AREA = 0.42;
const MAX_RENDERED_AMBIENT_EDGES = 120;
const MAX_RENDERED_SELECTED_EDGES = 40;

function dominantTag(tags: TagRelevance[]): string {
  if (tags.length === 0) return "bug";
  return tags.reduce((a, b) => (a.relevance > b.relevance ? a : b)).tag;
}

function issueRadius(tags: TagRelevance[]): number {
  const maxRelevance = Math.max(...tags.map((t) => t.relevance));
  return 6 + maxRelevance * 14;
}

// Check if a point is inside a polygon (ray casting)
function pointInPolygon(px: number, py: number, polygon: { x: number; y: number }[]): boolean {
  let inside = false;
  for (let i = 0, j = polygon.length - 1; i < polygon.length; j = i++) {
    const xi = polygon[i].x, yi = polygon[i].y;
    const xj = polygon[j].x, yj = polygon[j].y;
    if ((yi > py) !== (yj > py) && px < ((xj - xi) * (py - yi)) / (yj - yi) + xi) {
      inside = !inside;
    }
  }
  return inside;
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

function clampViewport(viewport: Viewport): Viewport {
  const width = clamp(viewport.xMax - viewport.xMin, MIN_VIEW_SIZE, MAX_VIEW_SIZE);
  const height = clamp(viewport.yMax - viewport.yMin, MIN_VIEW_SIZE, MAX_VIEW_SIZE);
  const centerX = (viewport.xMin + viewport.xMax) / 2;
  const centerY = (viewport.yMin + viewport.yMax) / 2;

  return {
    xMin: clamp(centerX - width / 2, -PAN_OVERSCAN, 1 + PAN_OVERSCAN - width),
    xMax: clamp(centerX + width / 2, -PAN_OVERSCAN + width, 1 + PAN_OVERSCAN),
    yMin: clamp(centerY - height / 2, -PAN_OVERSCAN, 1 + PAN_OVERSCAN - height),
    yMax: clamp(centerY + height / 2, -PAN_OVERSCAN + height, 1 + PAN_OVERSCAN),
  };
}

function viewportQuery(viewport: Viewport) {
  const params = new URLSearchParams({
    xMin: viewport.xMin.toFixed(4),
    xMax: viewport.xMax.toFixed(4),
    yMin: viewport.yMin.toFixed(4),
    yMax: viewport.yMax.toFixed(4),
  });
  return params.toString();
}

function clusterIntersectsViewport(cluster: MapCluster, viewport: Viewport) {
  return !(
    cluster.centerX + cluster.radius < viewport.xMin ||
    cluster.centerX - cluster.radius > viewport.xMax ||
    cluster.centerY + cluster.radius < viewport.yMin ||
    cluster.centerY - cluster.radius > viewport.yMax
  );
}

async function fetchJSON<T>(input: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(input, { signal });
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}`);
  }
  return res.json() as Promise<T>;
}

async function fetchViewportEdges(
  viewportKey: string,
  signal: AbortSignal
): Promise<EdgeData> {
  try {
    return await fetchJSON<EdgeData>(`/api/v1/map/edges?${viewportKey}`, signal);
  } catch (error) {
    if (!(error instanceof Error) || error.message !== "HTTP 404") {
      throw error;
    }

    const fallback = await fetchJSON<MapData>(`/api/v1/map?${viewportKey}`, signal);
    return { edges: fallback.edges };
  }
}

export default function MapPage() {
  const [mapData, setMapData] = useState<MapData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [viewport, setViewport] = useState<Viewport>(DEFAULT_VIEWPORT);
  const [loadedEdgeKey, setLoadedEdgeKey] = useState("");
  const [hoveredId, setHoveredId] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [lassoPoints, setLassoPoints] = useState<{ x: number; y: number }[]>([]);
  const [isLassoing, setIsLassoing] = useState(false);
  const [selectedBatch, setSelectedBatch] = useState<Set<string>>(new Set());
  const svgRef = useRef<SVGSVGElement>(null);
  const dragViewportRef = useRef<Viewport | null>(null);
  const panStartRef = useRef<{ x: number; y: number } | null>(null);
  const [dimensions, setDimensions] = useState({ width: 800, height: 600 });

  useEffect(() => {
    const initialViewportQuery = viewportQuery(DEFAULT_VIEWPORT);

    fetchJSON<MapData>(`/api/v1/map?${initialViewportQuery}`)
      .then((data: MapData) => {
        setMapData({ ...data, edges: EMPTY_EDGES });
        setLoading(false);
      })
      .catch((err) => {
        setError(err.message);
        setLoading(false);
      });
  }, []);

  const viewportKey = useMemo(() => viewportQuery(viewport), [viewport]);
  const viewportArea =
    (viewport.xMax - viewport.xMin) * (viewport.yMax - viewport.yMin);
  const shouldLoadAmbientEdges = viewportArea <= MAX_AMBIENT_VIEWPORT_AREA;
  const shouldRequestEdges = selectedId != null || shouldLoadAmbientEdges;

  useEffect(() => {
    if (!mapData) return;

    if (!shouldRequestEdges) {
      return;
    }

    if (viewportKey === loadedEdgeKey) {
      return;
    }

    const controller = new AbortController();
    const timeoutId = window.setTimeout(() => {
      fetchViewportEdges(viewportKey, controller.signal)
        .then((data: EdgeData) => {
          setMapData((current) =>
            current ? { ...current, edges: data.edges } : current
          );
          setLoadedEdgeKey(viewportKey);
        })
        .catch((err: Error & { name?: string }) => {
          if (err.name === "AbortError") return;
          console.error("failed to refresh map edges", err);
        });
    }, EDGE_FETCH_DEBOUNCE_MS);

    return () => {
      controller.abort();
      window.clearTimeout(timeoutId);
    };
  }, [loadedEdgeKey, mapData, shouldRequestEdges, viewportKey]);

  const containerRef = useCallback((node: HTMLDivElement | null) => {
    if (!node) return;
    const obs = new ResizeObserver((entries) => {
      const { width, height } = entries[0].contentRect;
      setDimensions({ width, height });
    });
    obs.observe(node);
    return () => obs.disconnect();
  }, []);

  const toScreen = useCallback((nx: number, ny: number) => {
    const width = viewport.xMax - viewport.xMin;
    const height = viewport.yMax - viewport.yMin;

    return {
      sx:
        PADDING +
        ((nx - viewport.xMin) / width) * (dimensions.width - PADDING * 2),
      sy:
        dimensions.height -
        PADDING -
        ((ny - viewport.yMin) / height) * (dimensions.height - PADDING * 2),
    };
  }, [dimensions.height, dimensions.width, viewport.xMax, viewport.xMin, viewport.yMax, viewport.yMin]);

  const toWorld = useCallback((sx: number, sy: number) => {
    const width = viewport.xMax - viewport.xMin;
    const height = viewport.yMax - viewport.yMin;
    const innerWidth = dimensions.width - PADDING * 2;
    const innerHeight = dimensions.height - PADDING * 2;
    const normalizedX = clamp((sx - PADDING) / innerWidth, 0, 1);
    const normalizedY = clamp((dimensions.height - PADDING - sy) / innerHeight, 0, 1);

    return {
      x: viewport.xMin + normalizedX * width,
      y: viewport.yMin + normalizedY * height,
    };
  }, [dimensions.height, dimensions.width, viewport.xMax, viewport.xMin, viewport.yMax, viewport.yMin]);

  function getSvgCoords(e: React.MouseEvent) {
    const svg = svgRef.current;
    if (!svg) return { x: 0, y: 0 };
    const rect = svg.getBoundingClientRect();
    return { x: e.clientX - rect.left, y: e.clientY - rect.top };
  }

  function getEmbeddingNeighbors(issueId: string): { id: string; similarity: number }[] {
    if (!mapData) return [];
    const candidateEdges =
      shouldRequestEdges && loadedEdgeKey === viewportKey
        ? edges
        : EMPTY_EDGES;
    const neighbors: { id: string; similarity: number }[] = [];
    for (const edge of candidateEdges) {
      if (edge.source === issueId) neighbors.push({ id: edge.target, similarity: edge.similarity });
      else if (edge.target === issueId) neighbors.push({ id: edge.source, similarity: edge.similarity });
    }
    return neighbors.sort((a, b) => b.similarity - a.similarity);
  }

  function getNeighborIds(issueId: string): Set<string> {
    return new Set(getEmbeddingNeighbors(issueId).map((n) => n.id));
  }

  const issues = mapData?.issues ?? EMPTY_ISSUES;
  const edges = mapData?.edges ?? EMPTY_EDGES;
  const clusters = mapData?.clusters ?? EMPTY_CLUSTERS;
  const hasCurrentEdges =
    shouldRequestEdges && loadedEdgeKey === viewportKey;
  const issueIndex = useMemo(
    () => new Map(issues.map((issue) => [issue.id, issue])),
    [issues]
  );

  const visibleIssueIds = useMemo(() => {
    const visible = new Set<string>();
    for (const issue of issues) {
      if (
        issue.x >= viewport.xMin &&
        issue.x <= viewport.xMax &&
        issue.y >= viewport.yMin &&
        issue.y <= viewport.yMax
      ) {
        visible.add(issue.id);
      }
    }
    return visible;
  }, [issues, viewport.xMax, viewport.xMin, viewport.yMax, viewport.yMin]);

  const visibleIssues = useMemo(
    () => issues.filter((issue) => visibleIssueIds.has(issue.id)),
    [issues, visibleIssueIds]
  );

  const visibleEdges = (hasCurrentEdges ? edges : EMPTY_EDGES).filter(
    (edge) =>
      visibleIssueIds.has(edge.source) ||
      visibleIssueIds.has(edge.target)
  );

  const rankedVisibleEdges = [...visibleEdges].sort((a, b) => {
    const aBothVisible =
      visibleIssueIds.has(a.source) && visibleIssueIds.has(a.target);
    const bBothVisible =
      visibleIssueIds.has(b.source) && visibleIssueIds.has(b.target);

    if (aBothVisible !== bBothVisible) {
      return aBothVisible ? -1 : 1;
    }

    if (a.similarity !== b.similarity) {
      return b.similarity - a.similarity;
    }

    return `${a.source}-${a.target}`.localeCompare(`${b.source}-${b.target}`);
  });

  const renderedEdges =
    selectedId != null
      ? rankedVisibleEdges
          .filter(
          (edge) =>
            edge.source === selectedId || edge.target === selectedId
        )
          .slice(0, MAX_RENDERED_SELECTED_EDGES)
      : shouldLoadAmbientEdges
      ? rankedVisibleEdges.slice(0, MAX_RENDERED_AMBIENT_EDGES)
      : EMPTY_EDGES;

  const visibleClusters = useMemo(
    () =>
      clusters.filter((cluster) => clusterIntersectsViewport(cluster, viewport)),
    [clusters, viewport]
  );

  // Lasso handlers
  function handleMouseDown(e: React.MouseEvent) {
    if ((e.target as SVGElement).closest("[data-issue]")) return;
    const coords = getSvgCoords(e);
    if (e.shiftKey) {
      setIsLassoing(true);
      setLassoPoints([coords]);
      setSelectedBatch(new Set());
      setSelectedId(null);
      return;
    }

    panStartRef.current = coords;
    dragViewportRef.current = viewport;
  }

  function handleMouseMove(e: React.MouseEvent) {
    const coords = getSvgCoords(e);
    if (isLassoing) {
      setLassoPoints((prev) => [...prev, coords]);
      return;
    }

    if (!panStartRef.current || !dragViewportRef.current) {
      return;
    }

    const innerWidth = dimensions.width - PADDING * 2;
    const innerHeight = dimensions.height - PADDING * 2;
    const startViewport = dragViewportRef.current;
    const dx =
      ((coords.x - panStartRef.current.x) / innerWidth) *
      (startViewport.xMax - startViewport.xMin);
    const dy =
      ((coords.y - panStartRef.current.y) / innerHeight) *
      (startViewport.yMax - startViewport.yMin);

    setViewport(
      clampViewport({
        xMin: startViewport.xMin - dx,
        xMax: startViewport.xMax - dx,
        yMin: startViewport.yMin + dy,
        yMax: startViewport.yMax + dy,
      })
    );
  }

  function handleMouseUp() {
    panStartRef.current = null;
    dragViewportRef.current = null;

    if (isLassoing) {
      setIsLassoing(false);

      if (lassoPoints.length < 3) {
        setLassoPoints([]);
        return;
      }

      const batch = new Set<string>();
      for (const issue of visibleIssues) {
        const { sx, sy } = toScreen(issue.x, issue.y);
        if (pointInPolygon(sx, sy, lassoPoints)) {
          batch.add(issue.id);
        }
      }
      setSelectedBatch(batch);
      if (batch.size === 0) setLassoPoints([]);
    }
  }

  function handleWheel(e: React.WheelEvent<SVGSVGElement>) {
    e.preventDefault();

    const coords = getSvgCoords(e);
    const cursor = toWorld(coords.x, coords.y);
    const zoomFactor = Math.exp(e.deltaY * 0.0015);
    const nextWidth = clamp(
      (viewport.xMax - viewport.xMin) * zoomFactor,
      MIN_VIEW_SIZE,
      MAX_VIEW_SIZE
    );
    const nextHeight = clamp(
      (viewport.yMax - viewport.yMin) * zoomFactor,
      MIN_VIEW_SIZE,
      MAX_VIEW_SIZE
    );
    const ratioX =
      (cursor.x - viewport.xMin) / (viewport.xMax - viewport.xMin);
    const ratioY =
      (cursor.y - viewport.yMin) / (viewport.yMax - viewport.yMin);

    setViewport(
      clampViewport({
        xMin: cursor.x - ratioX * nextWidth,
        xMax: cursor.x + (1 - ratioX) * nextWidth,
        yMin: cursor.y - ratioY * nextHeight,
        yMax: cursor.y + (1 - ratioY) * nextHeight,
      })
    );
  }

  const neighborIds =
    selectedId != null
      ? getNeighborIds(selectedId)
      : new Set<string>();

  const hasSelection = selectedId != null || selectedBatch.size > 0;

  function isHighlighted(id: string): boolean {
    if (selectedBatch.size > 0) return selectedBatch.has(id);
    if (selectedId != null) return id === selectedId || neighborIds.has(id);
    return true;
  }

  const activeIssue =
    issues.find((i) => i.id === (selectedId ?? hoveredId)) ?? null;

  const batchIssues = issues.filter((i) => selectedBatch.has(i.id));

  if (loading) {
    return (
      <SidebarProvider
        style={
          {
            "--sidebar-width": "calc(var(--spacing) * 72)",
            "--header-height": "calc(var(--spacing) * 12)",
          } as React.CSSProperties
        }
      >
        <AppSidebar variant="inset" />
        <SidebarInset>
          <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
            Loading map...
          </div>
        </SidebarInset>
      </SidebarProvider>
    );
  }

  if (error) {
    return (
      <SidebarProvider
        style={
          {
            "--sidebar-width": "calc(var(--spacing) * 72)",
            "--header-height": "calc(var(--spacing) * 12)",
          } as React.CSSProperties
        }
      >
        <AppSidebar variant="inset" />
        <SidebarInset>
          <div className="flex flex-1 items-center justify-center text-sm text-destructive">
            Failed to load map: {error}
          </div>
        </SidebarInset>
      </SidebarProvider>
    );
  }

  return (
    <SidebarProvider
      style={
        {
          "--sidebar-width": "calc(var(--spacing) * 72)",
          "--header-height": "calc(var(--spacing) * 12)",
        } as React.CSSProperties
      }
    >
      <AppSidebar variant="inset" />
      <SidebarInset>
        <header className="sticky top-0 z-10 shrink-0 border-b bg-background">
          <div className="flex items-center gap-2 px-4 h-(--header-height)">
            <h1 className="text-sm font-medium">Map</h1>
            <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground tabular-nums">
              {issues.length} issues
            </span>
            <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground tabular-nums">
              {visibleIssues.length} visible
            </span>
            <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground tabular-nums">
              {renderedEdges.length} edges
            </span>
            <button
              onClick={() => setViewport(DEFAULT_VIEWPORT)}
              className="text-[11px] text-muted-foreground hover:text-foreground"
            >
              Reset View
            </button>
            {selectedBatch.size > 0 && (
              <>
                <span className="rounded-full bg-primary px-2 py-0.5 text-[11px] font-medium text-primary-foreground">
                  {selectedBatch.size} selected
                </span>
                <button
                  onClick={() => {
                    setSelectedBatch(new Set());
                    setLassoPoints([]);
                  }}
                  className="text-[11px] text-muted-foreground hover:text-foreground"
                >
                  Clear
                </button>
              </>
            )}
            <span className="ml-auto text-[11px] text-muted-foreground/50">
              Scroll to zoom. Drag to pan. Shift-drag to lasso. Ambient edges appear once you zoom in far enough.
            </span>
          </div>
        </header>
        <div className="flex flex-1 flex-col overflow-hidden">
          <div ref={containerRef} className="relative flex-1 overflow-hidden">
            <svg
              ref={svgRef}
              width={dimensions.width}
              height={dimensions.height}
              className="absolute inset-0"
              style={{ cursor: isLassoing ? "crosshair" : "default" }}
              onMouseDown={handleMouseDown}
              onMouseMove={handleMouseMove}
              onMouseUp={handleMouseUp}
              onWheel={handleWheel}
              onMouseLeave={() => {
                handleMouseUp();
              }}
            >
              {/* Grid lines */}
              {[0.25, 0.5, 0.75].map((v) => {
                const { sx } = toScreen(v, 0);
                const { sy } = toScreen(0, v);
                return (
                  <g key={v}>
                    <line
                      x1={sx}
                      y1={PADDING}
                      x2={sx}
                      y2={dimensions.height - PADDING}
                      stroke="currentColor"
                      strokeOpacity={0.06}
                    />
                    <line
                      x1={PADDING}
                      y1={sy}
                      x2={dimensions.width - PADDING}
                      y2={sy}
                      stroke="currentColor"
                      strokeOpacity={0.06}
                    />
                  </g>
                );
              })}

              {/* Cluster labels */}
              {visibleClusters.map((cluster, i) => {
                const { sx, sy } = toScreen(cluster.centerX, cluster.centerY);
                const screenRadius =
                  (cluster.radius / (viewport.xMax - viewport.xMin)) *
                    (dimensions.width - PADDING * 2) +
                  40;
                return (
                  <g key={`cluster-${i}`}>
                    <circle
                      cx={sx}
                      cy={sy}
                      r={screenRadius}
                      fill={cluster.color}
                      fillOpacity={0.04}
                      stroke={cluster.color}
                      strokeOpacity={0.1}
                      strokeWidth={1}
                      strokeDasharray="4 4"
                    />
                    <text
                      x={sx}
                      y={sy - screenRadius - 6}
                      textAnchor="middle"
                      className="text-[10px] font-medium"
                      fill={cluster.color}
                      fillOpacity={0.5}
                    >
                      {cluster.label}
                    </text>
                  </g>
                );
              })}

              {/* Embedding similarity edges */}
              {renderedEdges.map((edge) => {
                const issueA = issueIndex.get(edge.source);
                const issueB = issueIndex.get(edge.target);
                if (!issueA || !issueB) return null;
                const { sx: x1, sy: y1 } = toScreen(issueA.x, issueA.y);
                const { sx: x2, sy: y2 } = toScreen(issueB.x, issueB.y);

                const isNeighborLink =
                  selectedId != null &&
                  ((edge.source === selectedId && neighborIds.has(edge.target)) ||
                    (edge.target === selectedId && neighborIds.has(edge.source)));
                const bothHighlighted =
                  isHighlighted(edge.source) && isHighlighted(edge.target);

                const baseOpacity = edge.similarity * 0.15;

                return (
                  <line
                    key={`${edge.source}-${edge.target}`}
                    x1={x1}
                    y1={y1}
                    x2={x2}
                    y2={y2}
                    stroke={
                      isNeighborLink
                        ? TAG_COLORS[dominantTag(
                            issueIndex.get(selectedId!)!.tags
                          )] ?? "#94a3b8"
                        : "currentColor"
                    }
                    strokeOpacity={
                      isNeighborLink
                        ? 0.3 + edge.similarity * 0.4
                        : hasSelection
                        ? bothHighlighted
                          ? baseOpacity * 2
                          : baseOpacity * 0.3
                        : baseOpacity
                    }
                    strokeWidth={isNeighborLink ? 1 + edge.similarity * 3 : 0.5 + edge.similarity * 2.5}
                  />
                );
              })}

              {/* Issue nodes */}
              {visibleIssues.map((issue) => {
                const { sx, sy } = toScreen(issue.x, issue.y);
                const color =
                  TAG_COLORS[dominantTag(issue.tags)] ?? "#94a3b8";
                const r = issueRadius(issue.tags);
                const isActive =
                  issue.id === hoveredId || issue.id === selectedId;
                const isNeighbor = neighborIds.has(issue.id);
                const highlighted = isHighlighted(issue.id);
                const dimmed = hasSelection && !highlighted;

                return (
                  <g
                    key={issue.id}
                    data-issue={issue.id}
                    onMouseEnter={() => setHoveredId(issue.id)}
                    onMouseLeave={() => setHoveredId(null)}
                    onClick={(e) => {
                      e.stopPropagation();
                      setSelectedBatch(new Set());
                      setLassoPoints([]);
                      setSelectedId(
                        selectedId === issue.id ? null : issue.id
                      );
                    }}
                    className="cursor-pointer"
                  >
                    {/* Neighbor pulse ring */}
                    {isNeighbor && selectedId != null && (
                      <circle
                        cx={sx}
                        cy={sy}
                        r={r + 6}
                        fill="none"
                        stroke={color}
                        strokeWidth={1.5}
                        strokeOpacity={0.3}
                        strokeDasharray="3 3"
                      />
                    )}
                    {/* Hover/select ring */}
                    {isActive && (
                      <circle
                        cx={sx}
                        cy={sy}
                        r={r + 4}
                        fill={color}
                        fillOpacity={0.15}
                      />
                    )}
                    {/* Batch selected ring */}
                    {selectedBatch.has(issue.id) && (
                      <circle
                        cx={sx}
                        cy={sy}
                        r={r + 5}
                        fill="none"
                        stroke={color}
                        strokeWidth={2}
                        strokeOpacity={0.6}
                      />
                    )}
                    <circle
                      cx={sx}
                      cy={sy}
                      r={r}
                      fill={color}
                      fillOpacity={dimmed ? 0.15 : isActive ? 0.9 : 0.6}
                      stroke={color}
                      strokeWidth={isActive ? 2 : 0}
                      strokeOpacity={0.8}
                    />
                    {(isActive || (isNeighbor && selectedId != null)) && (
                      <text
                        x={sx}
                        y={sy - r - (isNeighbor && !isActive ? 10 : 8)}
                        textAnchor="middle"
                        className="fill-foreground text-[11px] font-medium"
                        fillOpacity={isNeighbor && !isActive ? 0.6 : 1}
                      >
                        {issue.raw.length > 40
                          ? issue.raw.slice(0, 40) + "..."
                          : issue.raw}
                      </text>
                    )}
                  </g>
                );
              })}

              {/* Lasso path */}
              {lassoPoints.length > 1 && (
                <polygon
                  points={lassoPoints.map((p) => `${p.x},${p.y}`).join(" ")}
                  fill="currentColor"
                  fillOpacity={0.04}
                  stroke="currentColor"
                  strokeOpacity={0.3}
                  strokeWidth={1.5}
                  strokeDasharray="4 4"
                  strokeLinejoin="round"
                />
              )}
            </svg>

            {/* Detail sidebar — single issue */}
            <div
              className={`absolute right-0 top-0 h-full w-96 border-l bg-card shadow-lg transition-transform duration-200 ease-in-out ${
                activeIssue && selectedBatch.size === 0
                  ? "translate-x-0"
                  : "translate-x-full"
              }`}
            >
              {activeIssue && selectedBatch.size === 0 && (
                <div className="flex h-full flex-col overflow-y-auto p-5">
                  <div className="flex items-start justify-between gap-3">
                    <p className="whitespace-pre-wrap text-[13px] leading-relaxed">
                      {activeIssue.raw}
                    </p>
                    {selectedId && (
                      <button
                        onClick={() => setSelectedId(null)}
                        className="shrink-0 rounded-md p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
                      >
                        <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
                          <path d="M11 3L3 11M3 3l8 8" />
                        </svg>
                      </button>
                    )}
                  </div>
                  <div className="mt-5 space-y-2">
                    <p className="text-[11px] font-medium text-muted-foreground">
                      Tag Relevance
                    </p>
                    {activeIssue.tags.map(({ tag, relevance }) => (
                      <div key={tag} className="flex items-center gap-2">
                        <span className="w-20 text-[11px] text-muted-foreground">
                          {tag}
                        </span>
                        <div className="h-1.5 flex-1 rounded-full bg-muted">
                          <div
                            className="h-full rounded-full"
                            style={{
                              width: `${relevance * 100}%`,
                              backgroundColor:
                                TAG_COLORS[tag] ?? "#94a3b8",
                            }}
                          />
                        </div>
                        <span className="w-8 text-right text-[11px] tabular-nums text-muted-foreground">
                          {relevance.toFixed(1)}
                        </span>
                      </div>
                    ))}
                    {selectedId === activeIssue.id && (() => {
                      const neighbors = getEmbeddingNeighbors(activeIssue.id);
                      if (neighbors.length === 0) return null;
                      return (
                        <div className="mt-5 space-y-1.5">
                          <p className="text-[11px] font-medium text-muted-foreground">
                            Semantically Similar
                          </p>
                          {neighbors.map(({ id, similarity }) => {
                            const issue = issueIndex.get(id);
                            if (!issue) return null;
                            return (
                              <button
                                key={id}
                                className="flex w-full items-center gap-2 rounded-md px-1.5 py-1 -mx-1.5 text-left hover:bg-muted/60 transition-colors"
                                onClick={() => setSelectedId(id)}
                              >
                                <div
                                  className="h-2 w-2 shrink-0 rounded-full"
                                  style={{
                                    backgroundColor:
                                      TAG_COLORS[dominantTag(issue.tags)] ?? "#94a3b8",
                                  }}
                                />
                                <span className="flex-1 truncate text-[11px] text-muted-foreground">
                                  {issue.raw.length > 50
                                    ? issue.raw.slice(0, 50) + "..."
                                    : issue.raw}
                                </span>
                                <span className="shrink-0 text-[10px] tabular-nums text-muted-foreground/50">
                                  {(similarity * 100).toFixed(0)}%
                                </span>
                              </button>
                            );
                          })}
                        </div>
                      );
                    })()}
                  </div>
                </div>
              )}
            </div>

            {/* Batch sidebar — lasso selection */}
            <div
              className={`absolute right-0 top-0 h-full w-96 border-l bg-card shadow-lg transition-transform duration-200 ease-in-out ${
                selectedBatch.size > 0
                  ? "translate-x-0"
                  : "translate-x-full"
              }`}
            >
              {selectedBatch.size > 0 && (
                <div className="flex h-full flex-col overflow-y-auto p-5">
                  <div className="flex items-center justify-between">
                    <p className="text-[13px] font-medium">
                      Work Batch
                    </p>
                    <span className="rounded-full bg-primary px-2 py-0.5 text-[11px] font-medium text-primary-foreground">
                      {selectedBatch.size} issues
                    </span>
                  </div>
                  <div className="mt-4 space-y-2">
                    {batchIssues.map((issue) => (
                      <div
                        key={issue.id}
                        className="rounded-lg border border-border/40 p-2"
                      >
                        <p className="text-[12px] leading-snug">
                          {issue.raw.length > 80
                            ? issue.raw.slice(0, 80) + "..."
                            : issue.raw}
                        </p>
                        <div className="mt-1 flex gap-1">
                          {issue.tags
                            .filter((t) => t.relevance >= 0.6)
                            .map(({ tag }) => (
                              <span
                                key={tag}
                                className="rounded-full px-1.5 py-0.5 text-[9px] font-medium"
                                style={{
                                  backgroundColor: (TAG_COLORS[tag] ?? "#94a3b8") + "20",
                                  color: TAG_COLORS[tag] ?? "#94a3b8",
                                }}
                              >
                                {tag}
                              </span>
                            ))}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      </SidebarInset>
    </SidebarProvider>
  );
}
