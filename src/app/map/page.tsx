"use client";

import {
  startTransition,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { MouseEvent as ReactMouseEvent } from "react";
import { AppShell, AppShellToggle } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { apiURL } from "@/lib/api";

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

type ScreenPoint = {
  x: number;
  y: number;
};

type Neighbor = {
  id: string;
  similarity: number;
};

type TagComparison = {
  tag: string;
  average: number;
  minimum: number;
  maximum: number;
  spread: number;
};

type PairwiseTagSimilarity = {
  sourceId: string;
  targetId: string;
  similarity: number;
  sharedTags: TagComparison[];
};

type BatchAnalysis = {
  averageSimilarity: number;
  sharedTags: TagComparison[];
  supportingTags: TagComparison[];
  pairwise: PairwiseTagSimilarity[];
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
const EMPTY_NEIGHBORS: Neighbor[] = [];
const MIN_VIEW_SIZE = 0.12;
const MAX_VIEW_SIZE = 2.4;
const PAN_OVERSCAN = 0.35;
const EDGE_FETCH_DEBOUNCE_MS = 120;
const MIN_RENDERED_AMBIENT_EDGES = 24;
const RENDERED_EDGE_RATIO = 0.2;
const MAX_RENDERED_AMBIENT_EDGES = 180;
const MAX_RENDERED_SELECTED_EDGES = 40;

function dominantTag(tags: TagRelevance[]): string {
  if (tags.length === 0) return "bug";
  return tags.reduce((a, b) => (a.relevance > b.relevance ? a : b)).tag;
}

function issueRadius(tags: TagRelevance[]): number {
  if (tags.length === 0) return 6;
  const maxRelevance = Math.max(...tags.map((t) => t.relevance));
  return 6 + maxRelevance * 14;
}

function pointInPolygon(
  px: number,
  py: number,
  polygon: ScreenPoint[]
) {
  let inside = false;

  for (let i = 0, j = polygon.length - 1; i < polygon.length; j = i++) {
    const xi = polygon[i].x;
    const yi = polygon[i].y;
    const xj = polygon[j].x;
    const yj = polygon[j].y;

    if (
      (yi > py) !== (yj > py) &&
      px < ((xj - xi) * (py - yi)) / (yj - yi) + xi
    ) {
      inside = !inside;
    }
  }

  return inside;
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

function normalizeWheelDelta(delta: number, deltaMode: number, pageHeight: number) {
  if (deltaMode === 1) {
    return delta * 18;
  }

  if (deltaMode === 2) {
    return delta * pageHeight;
  }

  return delta;
}

function edgeRenderLimit(visibleNodeCount: number) {
  if (visibleNodeCount <= 0) return 0;

  return Math.min(
    MAX_RENDERED_AMBIENT_EDGES,
    Math.max(
      MIN_RENDERED_AMBIENT_EDGES,
      Math.ceil(visibleNodeCount * RENDERED_EDGE_RATIO)
    )
  );
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
  const res = await fetch(apiURL(input), { signal });

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

function issueLabel(raw: string, maxLength: number) {
  return raw.length > maxLength ? `${raw.slice(0, maxLength)}...` : raw;
}

function tagRelevanceMap(tags: TagRelevance[]): Record<string, number> {
  return Object.fromEntries(tags.map(({ tag, relevance }) => [tag, relevance]));
}

function cosineSimilarity(
  left: Record<string, number>,
  right: Record<string, number>,
  tags: string[]
) {
  let dot = 0;
  let leftMagnitude = 0;
  let rightMagnitude = 0;

  for (const tag of tags) {
    const a = left[tag] ?? 0;
    const b = right[tag] ?? 0;
    dot += a * b;
    leftMagnitude += a * a;
    rightMagnitude += b * b;
  }

  if (leftMagnitude === 0 || rightMagnitude === 0) {
    return 0;
  }

  return dot / (Math.sqrt(leftMagnitude) * Math.sqrt(rightMagnitude));
}

function analyzeBatch(batchIssues: MapIssue[]): BatchAnalysis | null {
  if (batchIssues.length < 2) {
    return null;
  }

  const loadingByIssue = batchIssues.map((issue) => ({
    issue,
    loadings: tagRelevanceMap(issue.tags),
  }));
  const tagUniverse = Array.from(
    new Set(batchIssues.flatMap((issue) => issue.tags.map(({ tag }) => tag)))
  );

  const tagComparisons = tagUniverse
    .map((tag) => {
      const values = loadingByIssue.map(({ loadings }) => loadings[tag] ?? 0);
      const average =
        values.reduce((sum, value) => sum + value, 0) / loadingByIssue.length;
      const minimum = Math.min(...values);
      const maximum = Math.max(...values);

      return {
        tag,
        average,
        minimum,
        maximum,
        spread: maximum - minimum,
      };
    })
    .sort((left, right) => {
      if (right.minimum !== left.minimum) {
        return right.minimum - left.minimum;
      }

      if (right.average !== left.average) {
        return right.average - left.average;
      }

      return left.tag.localeCompare(right.tag);
    });

  const pairwise: PairwiseTagSimilarity[] = [];
  for (let index = 0; index < loadingByIssue.length; index += 1) {
    for (let inner = index + 1; inner < loadingByIssue.length; inner += 1) {
      const source = loadingByIssue[index];
      const target = loadingByIssue[inner];

      pairwise.push({
        sourceId: source.issue.id,
        targetId: target.issue.id,
        similarity: cosineSimilarity(source.loadings, target.loadings, tagUniverse),
        sharedTags: tagComparisons
          .map((comparison) => {
            const sourceValue = source.loadings[comparison.tag] ?? 0;
            const targetValue = target.loadings[comparison.tag] ?? 0;

            return {
              tag: comparison.tag,
              average: (sourceValue + targetValue) / 2,
              minimum: Math.min(sourceValue, targetValue),
              maximum: Math.max(sourceValue, targetValue),
              spread: Math.abs(sourceValue - targetValue),
            };
          })
          .filter((comparison) => comparison.minimum >= 0.12)
          .sort((left, right) => {
            if (right.minimum !== left.minimum) {
              return right.minimum - left.minimum;
            }

            return right.average - left.average;
          })
          .slice(0, 3),
      });
    }
  }

  pairwise.sort((left, right) => right.similarity - left.similarity);

  return {
    averageSimilarity:
      pairwise.reduce((sum, pair) => sum + pair.similarity, 0) / pairwise.length,
    sharedTags: tagComparisons.filter((comparison) => comparison.minimum >= 0.12),
    supportingTags: tagComparisons.filter(
      (comparison) => comparison.average >= 0.2 && comparison.minimum < 0.12
    ),
    pairwise,
  };
}

export default function MapPage() {
  const [mapData, setMapData] = useState<MapData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [viewport, setViewport] = useState<Viewport>(DEFAULT_VIEWPORT);
  const [loadedEdgeKey, setLoadedEdgeKey] = useState("");
  const [hoveredId, setHoveredId] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [lassoPoints, setLassoPoints] = useState<ScreenPoint[]>([]);
  const [isLassoing, setIsLassoing] = useState(false);
  const [selectedBatch, setSelectedBatch] = useState<Set<string>>(new Set());
  const [showBatchAnalysis, setShowBatchAnalysis] = useState(false);
  const [dimensions, setDimensions] = useState({ width: 800, height: 600 });

  const containerRef = useRef<HTMLDivElement>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  const dragViewportRef = useRef<Viewport | null>(null);
  const panStartRef = useRef<ScreenPoint | null>(null);
  const viewportRef = useRef(DEFAULT_VIEWPORT);
  const lassoPointsRef = useRef<ScreenPoint[]>([]);
  const pendingViewportRef = useRef<Viewport | null>(null);
  const pendingLassoRef = useRef<ScreenPoint[] | null>(null);
  const viewportFrameRef = useRef<number | null>(null);
  const lassoFrameRef = useRef<number | null>(null);
  const blankClickCandidateRef = useRef(false);
  const blankClickStartRef = useRef<ScreenPoint | null>(null);

  useEffect(() => {
    viewportRef.current = viewport;
  }, [viewport]);

  useEffect(() => {
    lassoPointsRef.current = lassoPoints;
  }, [lassoPoints]);

  useEffect(() => {
    return () => {
      if (viewportFrameRef.current != null) {
        cancelAnimationFrame(viewportFrameRef.current);
      }
      if (lassoFrameRef.current != null) {
        cancelAnimationFrame(lassoFrameRef.current);
      }
    };
  }, []);

  useEffect(() => {
    const initialViewportQuery = viewportQuery(DEFAULT_VIEWPORT);

    fetchJSON<MapData>(`/api/v1/map?${initialViewportQuery}`)
      .then((data) => {
        setMapData(data);
        setLoadedEdgeKey(initialViewportQuery);
        setLoading(false);
      })
      .catch((caughtError) => {
        const message =
          caughtError instanceof Error ? caughtError.message : "Unknown error";
        setError(message);
        setLoading(false);
      });
  }, []);

  useEffect(() => {
    const node = containerRef.current;
    if (!node) return;
    const element = node;

    function measure() {
      const { width, height } = element.getBoundingClientRect();
      setDimensions((current) =>
        current.width === width && current.height === height
          ? current
          : { width, height }
      );
    }

    measure();

    const observer = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (!entry) return;

      const { width, height } = entry.contentRect;
      setDimensions((current) =>
        current.width === width && current.height === height
          ? current
          : { width, height }
      );
    });

    observer.observe(element);
    return () => observer.disconnect();
  }, [loading]);

  const viewportKey = useMemo(() => viewportQuery(viewport), [viewport]);

  useEffect(() => {
    if (!mapData || viewportKey === loadedEdgeKey) {
      return;
    }

    const controller = new AbortController();
    const timeoutId = window.setTimeout(() => {
      fetchViewportEdges(viewportKey, controller.signal)
        .then((data) => {
          setMapData((current) =>
            current ? { ...current, edges: data.edges } : current
          );
          setLoadedEdgeKey(viewportKey);
        })
        .catch((caughtError: Error & { name?: string }) => {
          if (caughtError.name === "AbortError") return;
          console.error("failed to refresh map edges", caughtError);
        });
    }, EDGE_FETCH_DEBOUNCE_MS);

    return () => {
      controller.abort();
      window.clearTimeout(timeoutId);
    };
  }, [loadedEdgeKey, mapData, viewportKey]);

  const issues = mapData?.issues ?? EMPTY_ISSUES;
  const edges = mapData?.edges ?? EMPTY_EDGES;
  const clusters = mapData?.clusters ?? EMPTY_CLUSTERS;
  const hasCurrentEdges = loadedEdgeKey === viewportKey;
  const currentEdges = hasCurrentEdges ? edges : EMPTY_EDGES;

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
  const ambientEdgeLimit = useMemo(
    () => edgeRenderLimit(visibleIssueIds.size),
    [visibleIssueIds]
  );

  const neighborMap = useMemo(() => {
    const next = new Map<string, Neighbor[]>();

    for (const edge of currentEdges) {
      const sourceNeighbors = next.get(edge.source) ?? [];
      sourceNeighbors.push({ id: edge.target, similarity: edge.similarity });
      next.set(edge.source, sourceNeighbors);

      const targetNeighbors = next.get(edge.target) ?? [];
      targetNeighbors.push({ id: edge.source, similarity: edge.similarity });
      next.set(edge.target, targetNeighbors);
    }

    for (const neighbors of next.values()) {
      neighbors.sort((a, b) => b.similarity - a.similarity);
    }

    return next;
  }, [currentEdges]);

  const selectedNeighbors = useMemo(
    () => (selectedId ? neighborMap.get(selectedId) ?? EMPTY_NEIGHBORS : EMPTY_NEIGHBORS),
    [neighborMap, selectedId]
  );

  const neighborIds = useMemo(
    () => new Set(selectedNeighbors.map((neighbor) => neighbor.id)),
    [selectedNeighbors]
  );

  const visibleEdges = useMemo(
    () =>
      currentEdges.filter(
        (edge) =>
          visibleIssueIds.has(edge.source) || visibleIssueIds.has(edge.target)
      ),
    [currentEdges, visibleIssueIds]
  );

  const rankedVisibleEdges = useMemo(() => {
    const ranked = [...visibleEdges];

    ranked.sort((a, b) => {
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

    return ranked;
  }, [visibleEdges, visibleIssueIds]);

  const renderedEdges = useMemo(() => {
    if (selectedId != null) {
      return rankedVisibleEdges
        .filter(
          (edge) => edge.source === selectedId || edge.target === selectedId
        )
        .slice(0, MAX_RENDERED_SELECTED_EDGES);
    }

    return rankedVisibleEdges.slice(0, ambientEdgeLimit);
  }, [ambientEdgeLimit, rankedVisibleEdges, selectedId]);

  const visibleClusters = useMemo(
    () =>
      clusters.filter((cluster) => clusterIntersectsViewport(cluster, viewport)),
    [clusters, viewport]
  );

  const selectedIssueColor = useMemo(() => {
    if (!selectedId) {
      return "#94a3b8";
    }

    const selectedIssue = issueIndex.get(selectedId);
    return TAG_COLORS[dominantTag(selectedIssue?.tags ?? [])] ?? "#94a3b8";
  }, [issueIndex, selectedId]);

  const activeIssue = useMemo(() => {
    const activeId = selectedId ?? hoveredId;
    return activeId ? issueIndex.get(activeId) ?? null : null;
  }, [hoveredId, issueIndex, selectedId]);

  const batchIssues = useMemo(
    () => issues.filter((issue) => selectedBatch.has(issue.id)),
    [issues, selectedBatch]
  );
  const batchAnalysis = useMemo(() => analyzeBatch(batchIssues), [batchIssues]);

  function scheduleViewport(nextViewport: Viewport) {
    viewportRef.current = nextViewport;
    pendingViewportRef.current = nextViewport;

    if (viewportFrameRef.current != null) {
      return;
    }

    viewportFrameRef.current = window.requestAnimationFrame(() => {
      const pendingViewport = pendingViewportRef.current;
      viewportFrameRef.current = null;
      pendingViewportRef.current = null;
      if (!pendingViewport) return;

      startTransition(() => {
        setViewport(pendingViewport);
      });
    });
  }

  function scheduleLassoPoints(nextPoints: ScreenPoint[]) {
    lassoPointsRef.current = nextPoints;
    pendingLassoRef.current = nextPoints;

    if (lassoFrameRef.current != null) {
      return;
    }

    lassoFrameRef.current = window.requestAnimationFrame(() => {
      const pendingPoints = pendingLassoRef.current;
      lassoFrameRef.current = null;
      pendingLassoRef.current = null;
      if (!pendingPoints) return;

      startTransition(() => {
        setLassoPoints(pendingPoints);
      });
    });
  }

  function clearSelection() {
    if (lassoFrameRef.current != null) {
      cancelAnimationFrame(lassoFrameRef.current);
      lassoFrameRef.current = null;
    }

    lassoPointsRef.current = [];
    pendingLassoRef.current = null;
    setShowBatchAnalysis(false);
    setSelectedBatch(new Set());
    setLassoPoints([]);
  }

  function clearAllSelections() {
    clearSelection();
    setSelectedId(null);
  }

  function toScreen(nx: number, ny: number) {
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
  }

  function getSvgCoords(event: { clientX: number; clientY: number }) {
    const svg = svgRef.current;
    if (!svg) return { x: 0, y: 0 };
    const rect = svg.getBoundingClientRect();
    return { x: event.clientX - rect.left, y: event.clientY - rect.top };
  }

  function toggleBatchIssue(issueId: string) {
    setShowBatchAnalysis(false);
    setSelectedId(null);
    setSelectedBatch((current) => {
      const next = new Set(current);

      if (current.size === 0 && selectedId != null) {
        next.add(selectedId);
      }

      if (next.has(issueId)) {
        next.delete(issueId);
      } else {
        next.add(issueId);
      }

      return next;
    });
  }

  useEffect(() => {
    const container = containerRef.current;
    const svg = svgRef.current;
    if (!container || !svg) return;

    function onWheel(event: WheelEvent) {
      const svg = svgRef.current;
      if (!svg) return;

      event.preventDefault();
      event.stopPropagation();
      blankClickCandidateRef.current = false;

      const currentViewport = viewportRef.current;
      const rect = svg.getBoundingClientRect();
      const coords = {
        x: event.clientX - rect.left,
        y: event.clientY - rect.top,
      };
      const viewportWidth = currentViewport.xMax - currentViewport.xMin;
      const viewportHeight = currentViewport.yMax - currentViewport.yMin;
      const innerWidth = dimensions.width - PADDING * 2;
      const innerHeight = dimensions.height - PADDING * 2;
      const cursor = {
        x:
          currentViewport.xMin +
          clamp((coords.x - PADDING) / innerWidth, 0, 1) * viewportWidth,
        y:
          currentViewport.yMin +
          clamp((dimensions.height - PADDING - coords.y) / innerHeight, 0, 1) *
            viewportHeight,
      };
      const normalizedDelta = normalizeWheelDelta(
        event.deltaY,
        event.deltaMode,
        dimensions.height
      );
      const clampedDelta = clamp(normalizedDelta, -72, 72);
      const zoomFactor = Math.exp(clampedDelta * 0.0011);
      const nextWidth = clamp(
        (currentViewport.xMax - currentViewport.xMin) * zoomFactor,
        MIN_VIEW_SIZE,
        MAX_VIEW_SIZE
      );
      const nextHeight = clamp(
        (currentViewport.yMax - currentViewport.yMin) * zoomFactor,
        MIN_VIEW_SIZE,
        MAX_VIEW_SIZE
      );
      const ratioX =
        (cursor.x - currentViewport.xMin) /
        (currentViewport.xMax - currentViewport.xMin);
      const ratioY =
        (cursor.y - currentViewport.yMin) /
        (currentViewport.yMax - currentViewport.yMin);

      scheduleViewport(
        clampViewport({
          xMin: cursor.x - ratioX * nextWidth,
          xMax: cursor.x + (1 - ratioX) * nextWidth,
          yMin: cursor.y - ratioY * nextHeight,
          yMax: cursor.y + (1 - ratioY) * nextHeight,
        })
      );
    }

    container.addEventListener("wheel", onWheel, { passive: false });
    return () => container.removeEventListener("wheel", onWheel);
  }, [dimensions.height, dimensions.width]);

  function isHighlighted(id: string): boolean {
    if (selectedBatch.size > 0) return selectedBatch.has(id);
    if (selectedId != null) return id === selectedId || neighborIds.has(id);
    return true;
  }

  function handleMouseDown(event: ReactMouseEvent<SVGSVGElement>) {
    if ((event.target as SVGElement).closest("[data-issue]")) return;

    const coords = getSvgCoords(event);
    blankClickCandidateRef.current = !event.shiftKey;
    blankClickStartRef.current = coords;

    if (event.shiftKey) {
      setIsLassoing(true);
      setShowBatchAnalysis(false);
      setSelectedBatch(new Set());
      setSelectedId(null);
      scheduleLassoPoints([coords]);
      return;
    }

    panStartRef.current = coords;
    dragViewportRef.current = viewportRef.current;
  }

  function handleMouseMove(event: ReactMouseEvent<SVGSVGElement>) {
    const coords = getSvgCoords(event);
    const clickStart = blankClickStartRef.current;

    if (
      clickStart &&
      Math.hypot(coords.x - clickStart.x, coords.y - clickStart.y) > 3
    ) {
      blankClickCandidateRef.current = false;
    }

    if (isLassoing) {
      scheduleLassoPoints([...lassoPointsRef.current, coords]);
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

    scheduleViewport(
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
    blankClickStartRef.current = null;

    if (!isLassoing) {
      return;
    }

    setIsLassoing(false);
    const points = lassoPointsRef.current;

    if (points.length < 3) {
      scheduleLassoPoints([]);
      return;
    }

    const batch = new Set<string>();
    for (const issue of visibleIssues) {
      const { sx, sy } = toScreen(issue.x, issue.y);
      if (pointInPolygon(sx, sy, points)) {
        batch.add(issue.id);
      }
    }

    setSelectedBatch(batch);
    if (batch.size === 0) {
      scheduleLassoPoints([]);
    }
  }

  function handleCanvasClick(event: ReactMouseEvent<SVGSVGElement>) {
    if ((event.target as SVGElement).closest("[data-issue]")) {
      blankClickCandidateRef.current = false;
      return;
    }

    if (!blankClickCandidateRef.current) {
      return;
    }

    blankClickCandidateRef.current = false;
    clearAllSelections();
  }

  const hasSelection = selectedId != null || selectedBatch.size > 0;

  if (loading) {
    return (
      <AppShell sidebar={<AppSidebar />}>
        <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
          Loading map...
        </div>
      </AppShell>
    );
  }

  if (error) {
    return (
      <AppShell sidebar={<AppSidebar />}>
        <div className="flex flex-1 items-center justify-center text-sm text-destructive">
          Failed to load map: {error}
        </div>
      </AppShell>
    );
  }

  return (
    <AppShell sidebar={<AppSidebar />}>
      <header className="sticky top-0 z-10 shrink-0 border-b bg-background">
        <div className="flex h-12 items-center gap-2 px-4">
          <AppShellToggle className="-ml-1" />
          <div className="mr-2 h-4 w-px shrink-0 bg-border" />
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
            type="button"
            onClick={() => scheduleViewport(DEFAULT_VIEWPORT)}
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
                type="button"
                onClick={clearSelection}
                className="text-[11px] text-muted-foreground hover:text-foreground"
              >
                Clear
              </button>
            </>
          )}
          <span className="ml-auto text-[11px] text-muted-foreground/50">
            Scroll to zoom. Drag to pan. Shift-click to build a batch. Shift-drag to lasso. Use Analyze in the sidebar to compare tag loadings.
          </span>
        </div>
      </header>
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <div
          ref={containerRef}
          className="relative min-h-0 flex-1 overflow-hidden overscroll-none"
        >
          <svg
            ref={svgRef}
            width={dimensions.width}
            height={dimensions.height}
            className="absolute inset-0 touch-none overscroll-none"
            style={{ cursor: isLassoing ? "crosshair" : "default" }}
            onMouseDown={handleMouseDown}
            onMouseMove={handleMouseMove}
            onMouseUp={handleMouseUp}
            onClick={handleCanvasClick}
            onMouseLeave={handleMouseUp}
          >
            {[0.25, 0.5, 0.75].map((value) => {
              const { sx } = toScreen(value, 0);
              const { sy } = toScreen(0, value);

              return (
                <g key={value}>
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

            {visibleClusters.map((cluster, index) => {
              const { sx, sy } = toScreen(cluster.centerX, cluster.centerY);
              const screenRadius =
                (cluster.radius / (viewport.xMax - viewport.xMin)) *
                  (dimensions.width - PADDING * 2) +
                40;

              return (
                <g key={`cluster-${index}`}>
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
              const baseOpacity = 0.16 + edge.similarity * 0.26;

              return (
                <line
                  key={`${edge.source}-${edge.target}`}
                  x1={x1}
                  y1={y1}
                  x2={x2}
                  y2={y2}
                  stroke={isNeighborLink ? selectedIssueColor : "currentColor"}
                  strokeOpacity={
                    isNeighborLink
                      ? 0.5 + edge.similarity * 0.35
                      : hasSelection
                        ? bothHighlighted
                          ? Math.min(baseOpacity * 1.5, 0.72)
                          : baseOpacity * 0.45
                        : baseOpacity
                  }
                  strokeWidth={
                    isNeighborLink ? 1 + edge.similarity * 3 : 0.5 + edge.similarity * 2.5
                  }
                />
              );
            })}

            {visibleIssues.map((issue) => {
              const { sx, sy } = toScreen(issue.x, issue.y);
              const color = TAG_COLORS[dominantTag(issue.tags)] ?? "#94a3b8";
              const radius = issueRadius(issue.tags);
              const isActive = issue.id === hoveredId || issue.id === selectedId;
              const isNeighbor = neighborIds.has(issue.id);
              const highlighted = isHighlighted(issue.id);
              const dimmed = hasSelection && !highlighted;

              return (
                <g
                  key={issue.id}
                  data-issue={issue.id}
                  onMouseEnter={() => setHoveredId(issue.id)}
                  onMouseLeave={() => setHoveredId(null)}
                  onClick={(event) => {
                    event.stopPropagation();

                    if (event.shiftKey) {
                      toggleBatchIssue(issue.id);
                      return;
                    }

                    clearSelection();
                    setSelectedId(selectedId === issue.id ? null : issue.id);
                  }}
                  className="cursor-pointer"
                >
                  {isNeighbor && selectedId != null && (
                    <circle
                      cx={sx}
                      cy={sy}
                      r={radius + 6}
                      fill="none"
                      stroke={color}
                      strokeWidth={1.5}
                      strokeOpacity={0.3}
                      strokeDasharray="3 3"
                    />
                  )}
                  {isActive && (
                    <circle
                      cx={sx}
                      cy={sy}
                      r={radius + 4}
                      fill={color}
                      fillOpacity={0.15}
                    />
                  )}
                  {selectedBatch.has(issue.id) && (
                    <circle
                      cx={sx}
                      cy={sy}
                      r={radius + 5}
                      fill="none"
                      stroke={color}
                      strokeWidth={2}
                      strokeOpacity={0.6}
                    />
                  )}
                  <circle
                    cx={sx}
                    cy={sy}
                    r={radius}
                    fill={color}
                    fillOpacity={dimmed ? 0.15 : isActive ? 0.9 : 0.6}
                    stroke={color}
                    strokeWidth={isActive ? 2 : 0}
                    strokeOpacity={0.8}
                  />
                  {(isActive || (isNeighbor && selectedId != null)) && (
                    <text
                      x={sx}
                      y={sy - radius - (isNeighbor && !isActive ? 10 : 8)}
                      textAnchor="middle"
                      className="fill-foreground text-[11px] font-medium"
                      fillOpacity={isNeighbor && !isActive ? 0.6 : 1}
                    >
                      {issue.raw.length > 40 ? `${issue.raw.slice(0, 40)}...` : issue.raw}
                    </text>
                  )}
                </g>
              );
            })}

            {lassoPoints.length > 1 && (
              <polygon
                points={lassoPoints.map((point) => `${point.x},${point.y}`).join(" ")}
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
                      type="button"
                      onClick={() => setSelectedId(null)}
                      className="shrink-0 rounded-md p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
                    >
                      <svg
                        width="14"
                        height="14"
                        viewBox="0 0 14 14"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="1.5"
                        strokeLinecap="round"
                      >
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
                            backgroundColor: TAG_COLORS[tag] ?? "#94a3b8",
                          }}
                        />
                      </div>
                      <span className="w-8 text-right text-[11px] tabular-nums text-muted-foreground">
                        {relevance.toFixed(1)}
                      </span>
                    </div>
                  ))}
                  {selectedId === activeIssue.id && selectedNeighbors.length > 0 && (
                    <div className="mt-5 space-y-1.5">
                      <p className="text-[11px] font-medium text-muted-foreground">
                        Semantically Similar
                      </p>
                      {selectedNeighbors.map(({ id, similarity }) => {
                        const issue = issueIndex.get(id);
                        if (!issue) return null;

                        return (
                          <button
                            key={id}
                            type="button"
                            className="flex w-full items-center gap-2 rounded-md px-1.5 py-1 text-left transition-colors hover:bg-muted/60"
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
                              {issueLabel(issue.raw, 50)}
                            </span>
                            <span className="shrink-0 text-[10px] tabular-nums text-muted-foreground/50">
                              {(similarity * 100).toFixed(0)}%
                            </span>
                          </button>
                        );
                      })}
                    </div>
                  )}
                </div>
              </div>
            )}
          </div>

          <div
            className={`absolute right-0 top-0 h-full w-96 border-l bg-card shadow-lg transition-transform duration-200 ease-in-out ${
              selectedBatch.size > 0 ? "translate-x-0" : "translate-x-full"
            }`}
          >
            {selectedBatch.size > 0 && (
              <div className="flex h-full flex-col overflow-y-auto p-5">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <p className="text-[13px] font-medium">
                      {showBatchAnalysis ? "Tag Loading Analysis" : "Work Batch"}
                    </p>
                    <p className="mt-1 text-[11px] text-muted-foreground">
                      {showBatchAnalysis
                        ? "Comparing the selected issues by their tag loadings."
                        : "Shift-click or shift-drag issues to build a comparison batch."}
                    </p>
                  </div>
                  <span className="rounded-full bg-primary px-2 py-0.5 text-[11px] font-medium text-primary-foreground">
                    {selectedBatch.size} issues
                  </span>
                </div>
                {showBatchAnalysis && batchAnalysis ? (
                  <div className="mt-4 space-y-5">
                    <div className="rounded-xl border border-border/50 bg-muted/20 p-3">
                      <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                        Average Pairwise Similarity
                      </p>
                      <p className="mt-2 text-2xl font-semibold tabular-nums">
                        {(batchAnalysis.averageSimilarity * 100).toFixed(0)}%
                      </p>
                    </div>

                    <div className="space-y-2">
                      <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                        Shared Tags
                      </p>
                      {(batchAnalysis.sharedTags.length > 0
                        ? batchAnalysis.sharedTags
                        : batchAnalysis.supportingTags
                      )
                        .slice(0, 6)
                        .map((comparison) => (
                          <div
                            key={comparison.tag}
                            className="rounded-lg border border-border/40 p-3"
                          >
                            <div className="flex items-center justify-between gap-3">
                              <span className="text-[12px] font-medium">
                                {comparison.tag}
                              </span>
                              <span className="text-[11px] tabular-nums text-muted-foreground">
                                {(comparison.minimum * 100).toFixed(0)}% shared
                              </span>
                            </div>
                            <div className="mt-2 h-1.5 rounded-full bg-muted">
                              <div
                                className="h-full rounded-full"
                                style={{
                                  width: `${comparison.average * 100}%`,
                                  backgroundColor:
                                    TAG_COLORS[comparison.tag] ?? "#94a3b8",
                                }}
                              />
                            </div>
                            <p className="mt-2 text-[11px] text-muted-foreground">
                              Avg {(comparison.average * 100).toFixed(0)}%, range{" "}
                              {(comparison.minimum * 100).toFixed(0)}-
                              {(comparison.maximum * 100).toFixed(0)}%
                            </p>
                          </div>
                        ))}
                    </div>

                    <div className="space-y-2">
                      <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                        Pairwise Breakdown
                      </p>
                      {batchAnalysis.pairwise.map((pair) => {
                        const source = issueIndex.get(pair.sourceId);
                        const target = issueIndex.get(pair.targetId);
                        if (!source || !target) return null;

                        return (
                          <div
                            key={`${pair.sourceId}-${pair.targetId}`}
                            className="rounded-lg border border-border/40 p-3"
                          >
                            <div className="flex items-center justify-between gap-3">
                              <p className="text-[12px] leading-snug">
                                {issueLabel(source.raw, 28)} /{" "}
                                {issueLabel(target.raw, 28)}
                              </p>
                              <span className="shrink-0 text-[11px] font-medium tabular-nums text-muted-foreground">
                                {(pair.similarity * 100).toFixed(0)}%
                              </span>
                            </div>
                            <div className="mt-2 flex flex-wrap gap-1">
                              {pair.sharedTags.length > 0 ? (
                                pair.sharedTags.map((comparison) => (
                                  <span
                                    key={comparison.tag}
                                    className="rounded-full px-1.5 py-0.5 text-[9px] font-medium"
                                    style={{
                                      backgroundColor: `${TAG_COLORS[comparison.tag] ?? "#94a3b8"}20`,
                                      color: TAG_COLORS[comparison.tag] ?? "#94a3b8",
                                    }}
                                  >
                                    {comparison.tag}{" "}
                                    {(comparison.minimum * 100).toFixed(0)}%
                                  </span>
                                ))
                              ) : (
                                <span className="text-[11px] text-muted-foreground">
                                  Low direct overlap in the strongest tags.
                                </span>
                              )}
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                ) : (
                  <div className="mt-4 space-y-2">
                    {batchIssues.map((issue) => (
                      <div
                        key={issue.id}
                        className="rounded-lg border border-border/40 p-2"
                      >
                        <p className="text-[12px] leading-snug">
                          {issueLabel(issue.raw, 80)}
                        </p>
                        <div className="mt-1 flex flex-wrap gap-1">
                          {issue.tags
                            .filter((tag) => tag.relevance >= 0.6)
                            .map(({ tag }) => (
                              <span
                                key={tag}
                                className="rounded-full px-1.5 py-0.5 text-[9px] font-medium"
                                style={{
                                  backgroundColor: `${TAG_COLORS[tag] ?? "#94a3b8"}20`,
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
                )}
                <div className="mt-4 flex gap-2">
                  {selectedBatch.size > 1 && (
                    <button
                      type="button"
                      onClick={() => setShowBatchAnalysis((current) => !current)}
                      className="rounded-md bg-foreground px-3 py-2 text-sm text-background transition-opacity hover:opacity-90"
                    >
                      {showBatchAnalysis ? "Back to Selection" : "Analyze"}
                    </button>
                  )}
                  <button
                    type="button"
                    onClick={clearAllSelections}
                    className="rounded-md border border-border/60 px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                  >
                    Clear Selection
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </AppShell>
  );
}
