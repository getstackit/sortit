"use client";

import {
  Suspense,
  startTransition,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { MouseEvent as ReactMouseEvent } from "react";
import { usePathname, useSearchParams } from "next/navigation";
import { AppShell } from "@/components/app-shell";
import {
  IssueMapCanvas,
  type IssueMapCanvasCluster,
  type IssueMapCanvasLine,
  type IssueMapCanvasNode,
} from "@/components/issue-map-canvas";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import { Switch } from "@/components/ui/switch";
import {
  compareIssueEmbeddings,
  fetchMapData,
  fetchViewportEdges,
} from "@/features/map/api";
import {
  analyzeBatch,
  clamp,
  clusterIntersectsViewport,
  dominantTag,
  edgeRenderLimit,
  issueRadius,
  MAX_RENDERED_SELECTED_EDGES,
  normalizeWheelDelta,
  pointInPolygon,
  TAG_COLORS,
} from "@/features/map/model";
import type {
  BatchEmbeddingAnalysis,
  MapCluster,
  MapData,
  MapIssue,
  Neighbor,
  ScreenPoint,
  Viewport,
} from "@/features/map/types";
import {
  clampViewport,
  DEFAULT_VIEWPORT,
  mapQuery,
  MAX_VIEW_SIZE,
  MIN_VIEW_SIZE,
  parseMapURLState,
  viewportQuery,
} from "@/features/map/url-state";

const PADDING = 60;
const EMPTY_ISSUES: MapIssue[] = [];
const EMPTY_EDGES: MapData["edges"] = [];
const EMPTY_CLUSTERS: MapCluster[] = [];
const EMPTY_NEIGHBORS: Neighbor[] = [];
const EDGE_FETCH_DEBOUNCE_MS = 120;

function issueLabel(raw: string, maxLength: number) {
  return raw.length > maxLength ? `${raw.slice(0, maxLength)}...` : raw;
}

function MapPageContent() {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const initialURLState = useMemo(() => parseMapURLState(searchParams), [searchParams]);
  const baseQueryParamsRef = useRef(new URLSearchParams(searchParams.toString()));
  const [mapData, setMapData] = useState<MapData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [viewport, setViewport] = useState<Viewport>(initialURLState.viewport);
  const [edgeThreshold, setEdgeThreshold] = useState(initialURLState.edgeThreshold);
  const [showClosed, setShowClosed] = useState(initialURLState.showClosed);
  const [loadedEdgeKey, setLoadedEdgeKey] = useState("");
  const [hoveredId, setHoveredId] = useState<string | null>(null);
  const [selectedIdState, setSelectedId] = useState<string | null>(
    initialURLState.selectedId
  );
  const [lassoPoints, setLassoPoints] = useState<ScreenPoint[]>([]);
  const [isLassoing, setIsLassoing] = useState(false);
  const [selectedBatchState, setSelectedBatch] = useState<Set<string>>(
    new Set(initialURLState.batchIds)
  );
  const [showBatchAnalysisState, setShowBatchAnalysis] = useState(
    initialURLState.showBatchAnalysis
  );
  const [batchEmbeddingAnalysis, setBatchEmbeddingAnalysis] =
    useState<BatchEmbeddingAnalysis | null>(null);
  const [batchEmbeddingError, setBatchEmbeddingError] = useState<string | null>(null);
  const [batchEmbeddingLoading, setBatchEmbeddingLoading] = useState(false);
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
  const showClosedRef = useRef(initialURLState.showClosed);
  const initialViewportKeyRef = useRef(
    mapQuery(
      initialURLState.viewport,
      initialURLState.edgeThreshold,
      initialURLState.showClosed
    )
  );

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
    const initialViewportQuery = initialViewportKeyRef.current;

    fetchMapData(initialViewportQuery)
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
    if (showClosedRef.current === showClosed) {
      return;
    }
    showClosedRef.current = showClosed;

    const controller = new AbortController();
    const query = mapQuery(viewport, edgeThreshold, showClosed);

    fetchMapData(query, controller.signal)
      .then((data) => {
        setMapData(data);
        setLoadedEdgeKey(query);
        setError(null);
      })
      .catch((caughtError: Error & { name?: string }) => {
        if (caughtError.name === "AbortError") {
          return;
        }

        const message =
          caughtError instanceof Error ? caughtError.message : "Unknown error";
        setError(message);
      });

    return () => controller.abort();
  }, [edgeThreshold, showClosed, viewport]);

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

  const viewportKey = useMemo(
    () => mapQuery(viewport, edgeThreshold, showClosed),
    [edgeThreshold, showClosed, viewport]
  );

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
  const closedIssueCount = useMemo(
    () => issues.filter((issue) => issue.status === "closed").length,
    [issues]
  );
  const hasCurrentEdges = loadedEdgeKey === viewportKey;
  const currentEdges = hasCurrentEdges ? edges : EMPTY_EDGES;
  const validIssueIDs = useMemo(
    () => new Set(issues.map((issue) => issue.id)),
    [issues]
  );
  const selectedId = useMemo(() => {
    if (!mapData || selectedIdState == null || validIssueIDs.has(selectedIdState)) {
      return selectedIdState;
    }

    return null;
  }, [mapData, selectedIdState, validIssueIDs]);
  const selectedBatch = useMemo(() => {
    if (!mapData) {
      return selectedBatchState;
    }

    const next = new Set<string>();
    for (const id of selectedBatchState) {
      if (validIssueIDs.has(id)) {
        next.add(id);
      }
    }
    return next;
  }, [mapData, selectedBatchState, validIssueIDs]);
  const selectedBatchKey = useMemo(
    () => Array.from(selectedBatch).sort().join(","),
    [selectedBatch]
  );
  const showBatchAnalysis = showBatchAnalysisState && selectedBatch.size > 1;

  useEffect(() => {
    const params = new URLSearchParams(baseQueryParamsRef.current?.toString() ?? "");
    params.delete("xMin");
    params.delete("xMax");
    params.delete("yMin");
    params.delete("yMax");
    params.delete("issue");
    params.delete("batch");
    params.delete("analyze");
    params.delete("edgeThreshold");
    params.delete("status");

    const viewportParams = new URLSearchParams(viewportQuery(viewport));
    for (const [key, value] of viewportParams.entries()) {
      params.set(key, value);
    }
    params.set("edgeThreshold", edgeThreshold.toFixed(2));
    if (showClosed) {
      params.set("status", "all");
    }

    if (selectedBatch.size > 0) {
      params.set("batch", selectedBatchKey);
      if (showBatchAnalysis && selectedBatch.size > 1) {
        params.set("analyze", "1");
      }
    } else if (selectedId != null) {
      params.set("issue", selectedId);
    }

    const nextSearch = params.toString();
    const nextURL = nextSearch ? `${pathname}?${nextSearch}` : pathname;
    const currentURL = `${window.location.pathname}${window.location.search}`;

    if (nextURL !== currentURL) {
      window.history.replaceState(window.history.state, "", nextURL);
    }
  }, [
    edgeThreshold,
    pathname,
    selectedBatch,
    selectedBatchKey,
    selectedId,
    showBatchAnalysis,
    showClosed,
    viewport,
  ]);

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

  useEffect(() => {
    if (!showBatchAnalysis || batchIssues.length < 2) {
      queueMicrotask(() => {
        setBatchEmbeddingAnalysis(null);
        setBatchEmbeddingError(null);
        setBatchEmbeddingLoading(false);
      });
      return;
    }

    const controller = new AbortController();
    queueMicrotask(() => {
      if (controller.signal.aborted) {
        return;
      }
      setBatchEmbeddingAnalysis(null);
      setBatchEmbeddingLoading(true);
      setBatchEmbeddingError(null);
    });

    compareIssueEmbeddings(
      batchIssues.map((issue) => issue.id),
      controller.signal
    )
      .then((payload) => {
        setBatchEmbeddingAnalysis(payload);
      })
      .catch((error) => {
        if (controller.signal.aborted) {
          return;
        }
        setBatchEmbeddingAnalysis(null);
        setBatchEmbeddingError(
          error instanceof Error ? error.message : "Failed to compare issues"
        );
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setBatchEmbeddingLoading(false);
        }
      });

    return () => controller.abort();
  }, [batchIssues, showBatchAnalysis]);

  const resetBatchEmbeddingAnalysis = useCallback(() => {
    setBatchEmbeddingAnalysis(null);
    setBatchEmbeddingError(null);
    setBatchEmbeddingLoading(false);
  }, []);

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

  const clearSelection = useCallback(() => {
    if (lassoFrameRef.current != null) {
      cancelAnimationFrame(lassoFrameRef.current);
      lassoFrameRef.current = null;
    }

    lassoPointsRef.current = [];
    pendingLassoRef.current = null;
    setShowBatchAnalysis(false);
    resetBatchEmbeddingAnalysis();
    setSelectedBatch(new Set());
    setLassoPoints([]);
  }, [resetBatchEmbeddingAnalysis]);

  const clearAllSelections = useCallback(() => {
    clearSelection();
    setSelectedId(null);
  }, [clearSelection]);

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

  const getSvgCoords = useCallback((event: { clientX: number; clientY: number }) => {
    const svg = svgRef.current;
    if (!svg) return { x: 0, y: 0 };
    const rect = svg.getBoundingClientRect();
    return { x: event.clientX - rect.left, y: event.clientY - rect.top };
  }, []);

  const toggleBatchIssue = useCallback((issueId: string) => {
    setShowBatchAnalysis(false);
    resetBatchEmbeddingAnalysis();
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
  }, [resetBatchEmbeddingAnalysis, selectedId]);

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

  const isHighlighted = useCallback((id: string): boolean => {
    if (selectedBatch.size > 0) return selectedBatch.has(id);
    if (selectedId != null) return id === selectedId || neighborIds.has(id);
    return true;
  }, [neighborIds, selectedBatch, selectedId]);

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

  const canvasGridLines = useMemo<IssueMapCanvasLine[]>(() => {
    return [0.25, 0.5, 0.75].flatMap((value) => {
      const { sx } = toScreen(value, 0);
      const { sy } = toScreen(0, value);

      return [
        {
          key: `grid-x-${value}`,
          x1: sx,
          y1: PADDING,
          x2: sx,
          y2: dimensions.height - PADDING,
          strokeOpacity: 0.06,
        },
        {
          key: `grid-y-${value}`,
          x1: PADDING,
          y1: sy,
          x2: dimensions.width - PADDING,
          y2: sy,
          strokeOpacity: 0.06,
        },
      ];
    });
  }, [toScreen, dimensions.height, dimensions.width]);

  const canvasClusters = useMemo<IssueMapCanvasCluster[]>(() => {
    return visibleClusters.map((cluster, index) => {
      const { sx, sy } = toScreen(cluster.centerX, cluster.centerY);
      const screenRadius =
        (cluster.radius / (viewport.xMax - viewport.xMin)) *
          (dimensions.width - PADDING * 2) +
        40;

      return {
        key: `cluster-${index}`,
        cx: sx,
        cy: sy,
        radius: screenRadius,
        fill: cluster.color,
        fillOpacity: 0.04,
        stroke: cluster.color,
        strokeOpacity: 0.1,
        strokeWidth: 1,
        strokeDasharray: "4 4",
        label: cluster.label,
        labelFill: cluster.color,
        labelFillOpacity: 0.5,
      };
    });
  }, [dimensions.width, toScreen, viewport.xMax, viewport.xMin, visibleClusters]);

  const canvasEdges = useMemo<IssueMapCanvasLine[]>(() => {
    return renderedEdges.flatMap((edge) => {
      const issueA = issueIndex.get(edge.source);
      const issueB = issueIndex.get(edge.target);
      if (!issueA || !issueB) {
        return [];
      }

      const { sx: x1, sy: y1 } = toScreen(issueA.x, issueA.y);
      const { sx: x2, sy: y2 } = toScreen(issueB.x, issueB.y);
      const isNeighborLink =
        selectedId != null &&
        ((edge.source === selectedId && neighborIds.has(edge.target)) ||
          (edge.target === selectedId && neighborIds.has(edge.source)));
      const bothHighlighted =
        isHighlighted(edge.source) && isHighlighted(edge.target);
      const baseOpacity = 0.16 + edge.similarity * 0.26;

      return [
        {
          key: `${edge.source}-${edge.target}`,
          x1,
          y1,
          x2,
          y2,
          stroke: isNeighborLink ? selectedIssueColor : "currentColor",
          strokeOpacity: isNeighborLink
            ? 0.5 + edge.similarity * 0.35
            : hasSelection
              ? bothHighlighted
                ? Math.min(baseOpacity * 1.5, 0.72)
                : baseOpacity * 0.45
              : baseOpacity,
          strokeWidth: isNeighborLink
            ? 1 + edge.similarity * 3
            : 0.5 + edge.similarity * 2.5,
        },
      ];
    });
  }, [
    hasSelection,
    issueIndex,
    neighborIds,
    renderedEdges,
    selectedId,
    selectedIssueColor,
    isHighlighted,
    toScreen,
  ]);

  const canvasNodes = useMemo<IssueMapCanvasNode[]>(() => {
    return visibleIssues.map((issue) => {
      const { sx, sy } = toScreen(issue.x, issue.y);
      const color = TAG_COLORS[dominantTag(issue.tags)] ?? "#94a3b8";
      const radius = issueRadius(issue.tags);
      const isActive = issue.id === hoveredId || issue.id === selectedId;
      const isNeighbor = neighborIds.has(issue.id);
      const highlighted = isHighlighted(issue.id);
      const dimmed = hasSelection && !highlighted;
      const isClosed = issue.status === "closed";
      const rings = [];

      if (isNeighbor && selectedId != null) {
        rings.push({
          radius: radius + 6,
          fill: "none",
          stroke: color,
          strokeWidth: 1.5,
          strokeOpacity: 0.3,
          strokeDasharray: "3 3",
        });
      }

      if (isActive) {
        rings.push({
          radius: radius + 4,
          fill: color,
          fillOpacity: 0.15,
        });
      }

      if (selectedBatch.has(issue.id)) {
        rings.push({
          radius: radius + 5,
          fill: "none",
          stroke: color,
          strokeWidth: 2,
          strokeOpacity: 0.6,
        });
      }

      return {
        key: issue.id,
        dataIssue: issue.id,
        cx: sx,
        cy: sy,
        radius,
        fill: color,
        fillOpacity: dimmed ? 0.15 : isClosed ? (isActive ? 0.42 : 0.22) : isActive ? 0.9 : 0.6,
        stroke: color,
        strokeWidth: isActive || isClosed ? 2 : 0,
        strokeOpacity: 0.8,
        strokeDasharray: isClosed ? "4 3" : undefined,
        rings,
        label:
          isActive || (isNeighbor && selectedId != null)
            ? issue.raw.length > 40
              ? `${issue.raw.slice(0, 40)}...`
              : issue.raw
            : undefined,
        labelY: sy - radius - (isNeighbor && !isActive ? 10 : 8),
        labelFillOpacity: isNeighbor && !isActive ? 0.6 : 1,
        className: "cursor-pointer",
        onMouseEnter: () => setHoveredId(issue.id),
        onMouseLeave: () => setHoveredId(null),
        onClick: (event) => {
          event.stopPropagation();

          if (event.shiftKey) {
            toggleBatchIssue(issue.id);
            return;
          }

          clearSelection();
          setSelectedId(selectedId === issue.id ? null : issue.id);
        },
      };
    });
  }, [
    hasSelection,
    hoveredId,
    neighborIds,
    selectedBatch,
    selectedId,
    clearSelection,
    isHighlighted,
    toScreen,
    toggleBatchIssue,
    visibleIssues,
  ]);

  if (loading) {
    return (
      <AppShell sidebar={<AppSidebar />}>
        <SiteHeader title="Map" />
        <div className="flex flex-1 items-center justify-center p-6">
          <div className="app-subtle-surface flex w-full max-w-xl items-center justify-center px-6 py-14 text-sm text-muted-foreground">
            Loading map...
          </div>
        </div>
      </AppShell>
    );
  }

  if (error) {
    return (
      <AppShell sidebar={<AppSidebar />}>
        <SiteHeader title="Map" />
        <div className="flex flex-1 items-center justify-center p-6">
          <div className="app-status-warning w-full max-w-xl">
            Failed to load map: {error}
          </div>
        </div>
      </AppShell>
    );
  }

  return (
    <AppShell sidebar={<AppSidebar />}>
      <SiteHeader
        title="Map"
        eyebrow="Explore"
        meta={
          <>
            <span className="app-chip tabular-nums">
              {issues.length} issues
            </span>
            {closedIssueCount > 0 && (
              <span className="app-chip tabular-nums">
                {closedIssueCount} closed
              </span>
            )}
            <span className="app-chip tabular-nums">
              {visibleIssues.length} visible
            </span>
            <span className="app-chip tabular-nums">
              {renderedEdges.length} edges
            </span>
          </>
        }
        actions={
          <>
            <label className="flex items-center gap-2 text-[11px] text-muted-foreground">
              <Switch
                checked={showClosed}
                onCheckedChange={(checked) => {
                  setShowClosed(checked);
                  setLoadedEdgeKey("");
                }}
                aria-label="Show closed issues"
              />
              <span>Show closed</span>
            </label>
            <label className="flex items-center gap-2 text-[11px] text-muted-foreground">
              <span>Threshold {Math.round(edgeThreshold * 100)}%</span>
              <input
                type="range"
                min="0"
                max="1"
                step="0.01"
                value={edgeThreshold}
                onChange={(event) => {
                  setEdgeThreshold(Number(event.target.value));
                  setLoadedEdgeKey("");
                }}
                className="h-1.5 w-28 accent-foreground"
                aria-label="Similarity edge threshold"
              />
            </label>
            <button
              type="button"
              onClick={() => scheduleViewport(DEFAULT_VIEWPORT)}
              className="text-[11px] text-muted-foreground hover:text-foreground"
            >
              Reset View
            </button>
            {selectedBatch.size > 0 && (
              <>
                <span className="rounded-full bg-primary px-2 py-0.5 text-[11px] font-medium text-primary-foreground shadow-sm">
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
            <span className="text-[11px] text-muted-foreground/50">
              Scroll to zoom. Drag to pan. Shift-click to build a batch. Shift-drag to lasso. Use Analyze in the sidebar to compare tags and embeddings.
            </span>
          </>
        }
      />
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <div
          ref={containerRef}
          className="relative min-h-0 flex-1 overflow-hidden overscroll-none"
        >
          <IssueMapCanvas
            ref={svgRef}
            width={dimensions.width}
            height={dimensions.height}
            className="absolute inset-0 touch-none overscroll-none"
            style={{ cursor: isLassoing ? "crosshair" : "default" }}
            background={
              <rect
                x="0"
                y="0"
                width={dimensions.width}
                height={dimensions.height}
                fill="transparent"
              />
            }
            onMouseDown={handleMouseDown}
            onMouseMove={handleMouseMove}
            onMouseUp={handleMouseUp}
            onClick={handleCanvasClick}
            onMouseLeave={handleMouseUp}
            gridLines={canvasGridLines}
            clusters={canvasClusters}
            edges={canvasEdges}
            nodes={canvasNodes}
          >
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
          </IssueMapCanvas>

          <div
            className={`absolute right-0 top-0 h-full w-96 border-l border-border/60 bg-card/88 shadow-lg backdrop-blur-xl transition-transform duration-200 ease-in-out ${
              activeIssue && selectedBatch.size === 0
                ? "translate-x-0"
                : "translate-x-full"
            }`}
          >
            {activeIssue && selectedBatch.size === 0 && (
              <div className="flex h-full flex-col overflow-y-auto p-5">
                <div className="flex items-start justify-between gap-3">
                  <div className="space-y-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="app-chip px-2 py-0.5 text-[10px]">
                        {activeIssue.id}
                      </span>
                      <span
                        className={
                          activeIssue.status === "closed"
                            ? "rounded-full bg-slate-200 px-2 py-0.5 text-[10px] font-medium text-slate-700"
                            : "rounded-full bg-emerald-100 px-2 py-0.5 text-[10px] font-medium text-emerald-700"
                        }
                      >
                        {activeIssue.status === "closed" ? "Closed" : "Open"}
                      </span>
                    </div>
                    <p className="whitespace-pre-wrap text-[13px] leading-relaxed">
                      {activeIssue.raw}
                    </p>
                  </div>
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
            className={`absolute right-0 top-0 h-full w-96 border-l border-border/60 bg-card/88 shadow-lg backdrop-blur-xl transition-transform duration-200 ease-in-out ${
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
                        ? "Comparing the selected issues by both tag loadings and embeddings."
                        : "Shift-click or shift-drag issues to build a comparison batch."}
                    </p>
                  </div>
                    <span className="rounded-full bg-primary px-2 py-0.5 text-[11px] font-medium text-primary-foreground shadow-sm">
                      {selectedBatch.size} issues
                    </span>
                </div>
                {showBatchAnalysis && batchAnalysis ? (
                  <div className="mt-4 space-y-5">
                    <div className="grid gap-3 md:grid-cols-2">
                      <div className="app-subtle-surface p-3">
                        <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                          Tag Similarity
                        </p>
                        <p className="mt-2 text-2xl font-semibold tabular-nums">
                          {(batchAnalysis.averageSimilarity * 100).toFixed(0)}%
                        </p>
                        <p className="mt-1 text-[11px] text-muted-foreground">
                          Based on selected issues&apos; tag loadings.
                        </p>
                      </div>
                      <div className="app-subtle-surface p-3">
                        <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                          Embedding Similarity
                        </p>
                        <p className="mt-2 text-2xl font-semibold tabular-nums">
                          {batchEmbeddingLoading
                            ? "..."
                            : batchEmbeddingAnalysis
                              ? `${(batchEmbeddingAnalysis.averageEmbeddingSimilarity * 100).toFixed(0)}%`
                              : "--"}
                        </p>
                        <p className="mt-1 text-[11px] text-muted-foreground">
                          Based on stored issue embeddings.
                        </p>
                      </div>
                    </div>

                    {batchEmbeddingError && (
                      <div className="app-status-warning">
                        {batchEmbeddingError}
                      </div>
                    )}

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
                            className="app-subtle-surface rounded-lg p-3"
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
                        const embeddingPair = batchEmbeddingAnalysis?.pairwiseEmbeddingSimilarity.find(
                          (item) =>
                            (item.sourceId === pair.sourceId &&
                              item.targetId === pair.targetId) ||
                            (item.sourceId === pair.targetId &&
                              item.targetId === pair.sourceId)
                        );

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
                              <div className="shrink-0 text-right text-[11px] font-medium tabular-nums text-muted-foreground">
                                <div>Tags {(pair.similarity * 100).toFixed(0)}%</div>
                                <div>
                                  Emb{" "}
                                  {embeddingPair
                                    ? `${(embeddingPair.similarity * 100).toFixed(0)}%`
                                    : batchEmbeddingLoading
                                      ? "..."
                                      : "--"}
                                </div>
                              </div>
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
                        className="app-subtle-surface rounded-lg p-2"
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
                      onClick={() =>
                        setShowBatchAnalysis((current) => {
                          const next = !current;
                          if (!next) {
                            resetBatchEmbeddingAnalysis();
                          }
                          return next;
                        })
                      }
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

function MapPageFallback() {
  return (
    <AppShell sidebar={<AppSidebar />}>
      <SiteHeader title="Map" eyebrow="Explore" />
      <div className="flex flex-1 items-center justify-center p-6">
        <div className="app-subtle-surface flex w-full max-w-xl items-center justify-center px-6 py-14 text-sm text-muted-foreground">
          Loading map...
        </div>
      </div>
    </AppShell>
  );
}

export default function MapPage() {
  return (
    <Suspense fallback={<MapPageFallback />}>
      <MapPageContent />
    </Suspense>
  );
}
