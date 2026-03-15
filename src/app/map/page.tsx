"use client";

import {
  Suspense,
  startTransition,
  useCallback,
  useDeferredValue,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";
import type { MouseEvent as ReactMouseEvent } from "react";
import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import { AppShell } from "@/components/app-shell";
import { IssueListItem } from "@/components/issue-list-item";
import {
  IssueMapCanvas,
  type IssueMapCanvasBlob,
  type IssueMapCanvasLine,
  type IssueMapCanvasNode,
  type IssueMapCanvasRing,
} from "@/components/issue-map-canvas";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import { TagRelevanceBars } from "@/components/tag-relevance-bars";
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
  computeBlobPath,
  dominantTag,
  edgeRenderLimit,
  issueRadius,
  MAX_RENDERED_SELECTED_EDGES,
  normalizeWheelDelta,
  pointInPolygon,
  TAG_COLORS,
  uniqueBlobColors,
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
import { Badge } from "@/components/ui/badge";
import { tagHref } from "@/lib/tags";

const PADDING = 60;
const EMPTY_ISSUES: MapIssue[] = [];
const EMPTY_EDGES: MapData["edges"] = [];
const EMPTY_CLUSTERS: MapCluster[] = [];
const EMPTY_NEIGHBORS: Neighbor[] = [];
const EDGE_FETCH_DEBOUNCE_MS = 120;

function viewportsEqual(left: Viewport, right: Viewport) {
  return (
    left.xMin === right.xMin &&
    left.xMax === right.xMax &&
    left.yMin === right.yMin &&
    left.yMax === right.yMax
  );
}

function issueIDSetsEqual(left: Set<string>, rightIDs: readonly string[]) {
  if (left.size !== rightIDs.length) {
    return false;
  }

  for (const id of rightIDs) {
    if (!left.has(id)) {
      return false;
    }
  }

  return true;
}

function issueLabel(raw: string, maxLength: number) {
  return raw.length > maxLength ? `${raw.slice(0, maxLength)}...` : raw;
}

function isMapSidebarScrollTarget(target: EventTarget | null) {
  if (target instanceof Element) {
    return target.closest("[data-map-sidebar-scroll]") != null;
  }

  if (target instanceof Node) {
    return target.parentElement?.closest("[data-map-sidebar-scroll]") != null;
  }

  return false;
}

function visibleIssueIDsForViewport(
  issues: readonly MapIssue[],
  viewport: Viewport
) {
  const visible: string[] = [];

  for (const issue of issues) {
    if (
      issue.x >= viewport.xMin &&
      issue.x <= viewport.xMax &&
      issue.y >= viewport.yMin &&
      issue.y <= viewport.yMax
    ) {
      visible.push(issue.id);
    }
  }

  return visible;
}

function edgeDataKeyForViewport(
  issues: readonly MapIssue[],
  viewport: Viewport,
  edgeThreshold: number,
  showClosed: boolean
) {
  return [
    showClosed ? "all" : "open",
    edgeThreshold.toFixed(2),
    visibleIssueIDsForViewport(issues, viewport).join(","),
  ].join("|");
}

function MapPageContent() {
  const canvasThemeID = useId().replace(/:/g, "");
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const searchParamsString = searchParams.toString();
  const urlState = parseMapURLState(searchParams);
  const baseQueryParamsRef = useRef(new URLSearchParams(searchParamsString));
  const [mapData, setMapData] = useState<MapData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [viewport, setViewport] = useState<Viewport>(urlState.viewport);
  const deferredViewport = useDeferredValue(viewport);
  const [edgeThreshold, setEdgeThreshold] = useState(urlState.edgeThreshold);
  const [showClosed, setShowClosed] = useState(urlState.showClosed);
  const [bubbleSizeTag, setBubbleSizeTag] = useState<string | null>(
    urlState.bubbleSizeTag
  );
  const [loadedEdgeDataKey, setLoadedEdgeDataKey] = useState("");
  const [hoveredId, setHoveredId] = useState<string | null>(null);
  const [selectedIdState, setSelectedId] = useState<string | null>(urlState.selectedId);
  const [lassoPoints, setLassoPoints] = useState<ScreenPoint[]>([]);
  const [isLassoing, setIsLassoing] = useState(false);
  const [selectedBatchState, setSelectedBatch] = useState<Set<string>>(
    new Set(urlState.batchIds)
  );
  const [showBatchAnalysisState, setShowBatchAnalysis] = useState(urlState.showBatchAnalysis);
  const [batchEmbeddingAnalysis, setBatchEmbeddingAnalysis] =
    useState<BatchEmbeddingAnalysis | null>(null);
  const [batchEmbeddingError, setBatchEmbeddingError] = useState<string | null>(null);
  const [batchEmbeddingLoading, setBatchEmbeddingLoading] = useState(false);
  const [dimensions, setDimensions] = useState({ width: 800, height: 600 });
  const [filteredClusterLabel, setFilteredClusterLabel] = useState<string | null>(null);

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
  const showClosedRef = useRef(urlState.showClosed);
  const initialViewportKeyRef = useRef(
    mapQuery(
      urlState.viewport,
      urlState.edgeThreshold,
      urlState.showClosed
    )
  );

  useEffect(() => {
    viewportRef.current = viewport;
  }, [viewport]);

  useEffect(() => {
    lassoPointsRef.current = lassoPoints;
  }, [lassoPoints]);

  useEffect(() => {
    baseQueryParamsRef.current = new URLSearchParams(searchParamsString);

    startTransition(() => {
      setViewport((current) =>
        viewportsEqual(current, urlState.viewport) ? current : urlState.viewport
      );
      setEdgeThreshold((current) =>
        current === urlState.edgeThreshold ? current : urlState.edgeThreshold
      );
      setShowClosed((current) =>
        current === urlState.showClosed ? current : urlState.showClosed
      );
      setBubbleSizeTag((current) =>
        current === urlState.bubbleSizeTag ? current : urlState.bubbleSizeTag
      );
      setSelectedId((current) =>
        current === urlState.selectedId ? current : urlState.selectedId
      );
      setSelectedBatch((current) =>
        issueIDSetsEqual(current, urlState.batchIds)
          ? current
          : new Set(urlState.batchIds)
      );
      setShowBatchAnalysis((current) =>
        current === urlState.showBatchAnalysis ? current : urlState.showBatchAnalysis
      );
    });
  }, [
    searchParams,
    searchParamsString,
    urlState.batchIds,
    urlState.edgeThreshold,
    urlState.bubbleSizeTag,
    urlState.selectedId,
    urlState.showBatchAnalysis,
    urlState.showClosed,
    urlState.viewport,
  ]);

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
    const controller = new AbortController();
    const query = initialViewportKeyRef.current;

    fetchMapData(query, controller.signal)
      .then((data) => {
        setMapData(data);
        setLoadedEdgeDataKey(
          edgeDataKeyForViewport(
            data.issues,
            urlState.viewport,
            urlState.edgeThreshold,
            urlState.showClosed
          )
        );
        setLoading(false);
      })
      .catch((caughtError: Error & { name?: string }) => {
        if (caughtError.name === "AbortError") return;
        const message =
          caughtError instanceof Error ? caughtError.message : "Unknown error";
        setError(message);
        setLoading(false);
      });

    return () => controller.abort();
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
        setLoadedEdgeDataKey(
          edgeDataKeyForViewport(
            data.issues,
            viewport,
            edgeThreshold,
            showClosed
          )
        );
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
  const issues = mapData?.issues ?? EMPTY_ISSUES;
  const edges = mapData?.edges ?? EMPTY_EDGES;
  const clusters = mapData?.clusters ?? EMPTY_CLUSTERS;
  const edgeDataKey = useMemo(
    () => edgeDataKeyForViewport(issues, viewport, edgeThreshold, showClosed),
    [edgeThreshold, issues, showClosed, viewport]
  );

  useEffect(() => {
    if (!mapData || edgeDataKey === loadedEdgeDataKey) {
      return;
    }

    const controller = new AbortController();
    const timeoutId = window.setTimeout(() => {
      fetchViewportEdges(viewportKey, controller.signal)
        .then((data) => {
          setMapData((current) =>
            current ? { ...current, edges: data.edges } : current
          );
          setLoadedEdgeDataKey(edgeDataKey);
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
  }, [edgeDataKey, loadedEdgeDataKey, mapData, viewportKey]);

  const closedIssueCount = useMemo(
    () => issues.filter((issue) => issue.status === "closed").length,
    [issues]
  );
  const hasCurrentEdges = loadedEdgeDataKey === edgeDataKey;
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
    params.delete("sizeTag");

    const viewportParams = new URLSearchParams(viewportQuery(viewport));
    for (const [key, value] of viewportParams.entries()) {
      params.set(key, value);
    }
    params.set("edgeThreshold", edgeThreshold.toFixed(2));
    if (showClosed) {
      params.set("status", "all");
    }
    if (bubbleSizeTag) {
      params.set("sizeTag", bubbleSizeTag);
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
    bubbleSizeTag,
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

  const visibleIssueIds = useMemo(
    () => new Set(visibleIssueIDsForViewport(issues, deferredViewport)),
    [issues, deferredViewport]
  );

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

  const sortedEdges = useMemo(() => {
    const sorted = [...currentEdges];
    sorted.sort((a, b) => {
      if (a.similarity !== b.similarity) return b.similarity - a.similarity;
      return `${a.source}-${a.target}`.localeCompare(`${b.source}-${b.target}`);
    });
    return sorted;
  }, [currentEdges]);

  const renderedEdges = useMemo(() => {
    if (selectedId != null) {
      const selected: typeof sortedEdges = [];
      for (const edge of sortedEdges) {
        if (edge.source === selectedId || edge.target === selectedId) {
          selected.push(edge);
          if (selected.length >= MAX_RENDERED_SELECTED_EDGES) break;
        }
      }
      return selected;
    }

    const bothVisible: typeof sortedEdges = [];
    const oneVisible: typeof sortedEdges = [];

    for (const edge of sortedEdges) {
      const srcIn = visibleIssueIds.has(edge.source);
      const tgtIn = visibleIssueIds.has(edge.target);
      if (!srcIn && !tgtIn) continue;

      if (srcIn && tgtIn) {
        bothVisible.push(edge);
      } else {
        oneVisible.push(edge);
      }

      if (bothVisible.length + oneVisible.length >= ambientEdgeLimit) break;
    }

    const result = bothVisible.concat(oneVisible);
    return result.slice(0, ambientEdgeLimit);
  }, [ambientEdgeLimit, sortedEdges, selectedId, visibleIssueIds]);

  const visibleClusters = useMemo(
    () =>
      clusters.filter((cluster) => clusterIntersectsViewport(cluster, deferredViewport)),
    [clusters, deferredViewport]
  );

  const selectedIssueColor = useMemo(() => {
    if (!selectedId) {
      return "#94a3b8";
    }

    const selectedIssue = issueIndex.get(selectedId);
    return TAG_COLORS[dominantTag(selectedIssue?.tags ?? [])] ?? "#94a3b8";
  }, [issueIndex, selectedId]);

  const sidebarIssue = useMemo(() => {
    if (selectedId != null) {
      return issueIndex.get(selectedId) ?? null;
    }

    if (filteredClusterLabel != null) {
      return null;
    }

    return hoveredId ? issueIndex.get(hoveredId) ?? null : null;
  }, [filteredClusterLabel, hoveredId, issueIndex, selectedId]);

  const batchIssues = useMemo(
    () => issues.filter((issue) => selectedBatch.has(issue.id)),
    [issues, selectedBatch]
  );
  const batchAnalysis = useMemo(() => analyzeBatch(batchIssues), [batchIssues]);
  const bubbleSizeOptions = useMemo(() => {
    const aggregate = new Map<string, { total: number; count: number }>();

    for (const issue of issues) {
      for (const { tag, relevance } of issue.tags) {
        const current = aggregate.get(tag) ?? { total: 0, count: 0 };
        current.total += relevance;
        current.count += 1;
        aggregate.set(tag, current);
      }
    }

    const options = Array.from(aggregate.entries())
      .map(([tag, value]) => ({
        tag,
        average: value.total / value.count,
        count: value.count,
      }))
      .sort((left, right) => {
        if (right.count !== left.count) {
          return right.count - left.count;
        }
        if (right.average !== left.average) {
          return right.average - left.average;
        }
        return left.tag.localeCompare(right.tag);
      });

    if (
      bubbleSizeTag &&
      !options.some((option) => option.tag === bubbleSizeTag)
    ) {
      options.unshift({ tag: bubbleSizeTag, average: 0, count: 0 });
    }

    return options;
  }, [bubbleSizeTag, issues]);

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
  }, [setBatchEmbeddingAnalysis, setBatchEmbeddingError, setBatchEmbeddingLoading]);

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
  }, [resetBatchEmbeddingAnalysis, setShowBatchAnalysis, setSelectedBatch, setLassoPoints]);

  const clearAllSelections = useCallback(() => {
    clearSelection();
    setSelectedId(null);
    setFilteredClusterLabel(null);
  }, [clearSelection, setSelectedId, setFilteredClusterLabel]);

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
  }, [resetBatchEmbeddingAnalysis, selectedId, setShowBatchAnalysis, setSelectedId, setSelectedBatch]);

  useEffect(() => {
    const container = containerRef.current;
    const svg = svgRef.current;
    if (!container || !svg) return;

    function onWheel(event: WheelEvent) {
      const svg = svgRef.current;
      if (!svg) return;
      if (isMapSidebarScrollTarget(event.target)) return;

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
    const issueEl = (event.target as SVGElement).closest("[data-issue]");
    if (issueEl) {
      const issueId = issueEl.getAttribute("data-issue");
      if (!issueId) return;
      blankClickCandidateRef.current = false;

      if (event.shiftKey) {
        toggleBatchIssue(issueId);
      } else {
        clearSelection();
        setSelectedId(selectedId === issueId ? null : issueId);
      }
      return;
    }

    if (!blankClickCandidateRef.current) {
      return;
    }

    blankClickCandidateRef.current = false;
    clearAllSelections();
  }

  const handleMouseOver = useCallback((event: ReactMouseEvent<SVGSVGElement>) => {
    const issueEl = (event.target as SVGElement).closest("[data-issue]");
    if (issueEl) {
      const issueId = issueEl.getAttribute("data-issue");
      if (issueId) setHoveredId(issueId);
    }
  }, [setHoveredId]);

  const handleMouseOut = useCallback((event: ReactMouseEvent<SVGSVGElement>) => {
    const issueEl = (event.target as SVGElement).closest("[data-issue]");
    if (!issueEl) return;
    const related = event.relatedTarget as SVGElement | null;
    if (related && issueEl.contains(related)) return;
    setHoveredId(null);
  }, [setHoveredId]);

  const hasSelection = selectedId != null || selectedBatch.size > 0;

  const canvasGridLines = useMemo<IssueMapCanvasLine[]>(() => {
    return [0.15, 0.3, 0.5, 0.7, 0.85].flatMap((value) => {
      const { sx } = toScreen(value, 0);
      const { sy } = toScreen(0, value);
      const isCenterLine = value === 0.5;

      return [
        {
          key: `grid-x-${value}`,
          x1: sx,
          y1: PADDING,
          x2: sx,
          y2: dimensions.height - PADDING,
          strokeOpacity: isCenterLine ? 0.14 : 0.07,
          strokeWidth: isCenterLine ? 1.4 : 1,
        },
        {
          key: `grid-y-${value}`,
          x1: PADDING,
          y1: sy,
          x2: dimensions.width - PADDING,
          y2: sy,
          strokeOpacity: isCenterLine ? 0.14 : 0.07,
          strokeWidth: isCenterLine ? 1.4 : 1,
        },
      ];
    });
  }, [toScreen, dimensions.height, dimensions.width]);

  const clusterMembers = useMemo(() => {
    const result = new Map<string, string[]>();
    for (const cluster of clusters) {
      if (cluster.issueIds && cluster.issueIds.length > 0) {
        result.set(cluster.label, cluster.issueIds);
      } else {
        const searchRadius = cluster.radius * 1.3 + 0.04;
        const radiusSq = searchRadius * searchRadius;
        const members = issues
          .filter((issue) => {
            const dx = issue.x - cluster.centerX;
            const dy = issue.y - cluster.centerY;
            return dx * dx + dy * dy <= radiusSq;
          })
          .map((issue) => issue.id);
        result.set(cluster.label, members);
      }
    }
    return result;
  }, [clusters, issues]);

  const canvasBlobs = useMemo<IssueMapCanvasBlob[]>(() => {
    const blobColorByKey = uniqueBlobColors(
      visibleClusters.map(
        (cluster, clusterIndex) =>
          `${cluster.label}::${cluster.topTag ?? ""}::${clusterIndex}`
      )
    );

    return visibleClusters
      .filter((cluster) => {
        const members = clusterMembers.get(cluster.label) ?? [];
        const minSize = cluster.issueIds ? 5 : 3;
        return members.length >= minSize;
      })
      .map((cluster, clusterIndex) => {
        const memberIds = clusterMembers.get(cluster.label) ?? [];
        const memberPoints = memberIds
          .map((id) => issueIndex.get(id))
          .filter((issue): issue is MapIssue => issue != null)
          .map((issue) => toScreen(issue.x, issue.y))
          .map(({ sx, sy }) => ({ x: sx, y: sy }));

        const path = computeBlobPath(memberPoints, 45);
        if (!path) return null;

        const centroidX = memberPoints.reduce((s, p) => s + p.x, 0) / memberPoints.length;
        const topY = Math.min(...memberPoints.map((p) => p.y));
        const blobColorKey = `${cluster.label}::${cluster.topTag ?? ""}::${clusterIndex}`;
        const color = blobColorByKey[blobColorKey] ?? "#94a3b8";
        const isFiltered = filteredClusterLabel != null;
        const isThisCluster = filteredClusterLabel === cluster.label;

        return {
          key: `blob-${clusterIndex}-${cluster.label}`,
          path,
          fill: color,
          fillOpacity: isFiltered ? (isThisCluster ? 0.18 : 0.035) : 0.13,
          stroke: color,
          strokeOpacity: isFiltered ? (isThisCluster ? 0.55 : 0.12) : 0.3,
          strokeWidth: isFiltered ? (isThisCluster ? 2.2 : 1.1) : 1.4,
          filter: `url(#map-blob-shadow-${canvasThemeID})`,
          label: cluster.label,
          labelX: centroidX,
          labelY: Math.max(topY - 28, PADDING - 12),
          labelClassName: "text-[11px] font-semibold uppercase tracking-[0.2em]",
          labelStrokeWidth: 7,
          labelFill: color,
          labelFillOpacity: isFiltered ? (isThisCluster ? 0.88 : 0.24) : 0.74,
          onClick: (e: React.MouseEvent<SVGGElement>) => {
            e.stopPropagation();
            setFilteredClusterLabel(
              filteredClusterLabel === cluster.label ? null : cluster.label
            );
          },
          className: "cursor-pointer",
        };
      })
      .filter(Boolean) as IssueMapCanvasBlob[];
  }, [
    canvasThemeID,
    visibleClusters,
    clusterMembers,
    issueIndex,
    toScreen,
    filteredClusterLabel,
    setFilteredClusterLabel,
  ]);

  const filteredIssueIds = useMemo(() => {
    if (!filteredClusterLabel) return null;
    const members = clusterMembers.get(filteredClusterLabel);
    if (!members || members.length === 0) return null;
    return new Set(members);
  }, [filteredClusterLabel, clusterMembers]);

  const visibleTagLegend = useMemo(() => {
    const counts = new Map<string, number>();

    for (const issue of visibleIssues) {
      const tag = dominantTag(issue.tags);
      counts.set(tag, (counts.get(tag) ?? 0) + 1);
    }

    return Array.from(counts.entries())
      .map(([tag, count]) => ({
        tag,
        count,
        color: TAG_COLORS[tag] ?? "#94a3b8",
      }))
      .sort((left, right) => {
        if (right.count !== left.count) {
          return right.count - left.count;
        }

        return left.tag.localeCompare(right.tag);
      })
      .slice(0, 5);
  }, [visibleIssues]);

  const selectedCluster = useMemo(() => {
    if (!filteredClusterLabel) {
      return null;
    }

    return clusters.find((cluster) => cluster.label === filteredClusterLabel) ?? null;
  }, [clusters, filteredClusterLabel]);

  const selectedClusterIssues = useMemo(() => {
    if (!filteredClusterLabel) {
      return [];
    }

    const memberIds = clusterMembers.get(filteredClusterLabel) ?? [];
    const focusTag = selectedCluster?.topTag ?? null;
    return memberIds
      .map((id) => issueIndex.get(id))
      .filter((issue): issue is MapIssue => issue != null)
      .sort((left, right) => {
        if (left.status !== right.status) {
          return left.status === "open" ? -1 : 1;
        }

        const leftRelevance =
          (focusTag
            ? left.tags.find((entry) => entry.tag === focusTag)?.relevance
            : undefined) ??
          left.tags[0]?.relevance ??
          0;
        const rightRelevance =
          (focusTag
            ? right.tags.find((entry) => entry.tag === focusTag)?.relevance
            : undefined) ??
          right.tags[0]?.relevance ??
          0;

        return rightRelevance - leftRelevance;
      });
  }, [clusterMembers, filteredClusterLabel, issueIndex, selectedCluster]);

  const selectedClusterTagProfile = useMemo(() => {
    if (selectedClusterIssues.length === 0) {
      return [];
    }

    const aggregate = new Map<string, { total: number; count: number }>();
    for (const issue of selectedClusterIssues) {
      for (const { tag, relevance } of issue.tags) {
        const current = aggregate.get(tag) ?? { total: 0, count: 0 };
        current.total += relevance;
        current.count += 1;
        aggregate.set(tag, current);
      }
    }

    return Array.from(aggregate.entries())
      .map(([tag, value]) => ({
        tag,
        relevance: value.total / value.count,
      }))
      .sort((left, right) => right.relevance - left.relevance)
      .slice(0, 6);
  }, [selectedClusterIssues]);

  const selectedClusterOpenCount = useMemo(
    () => selectedClusterIssues.filter((issue) => issue.status === "open").length,
    [selectedClusterIssues]
  );

  const staticEdgeData = useMemo(() => {
    return renderedEdges.flatMap((edge) => {
      const issueA = issueIndex.get(edge.source);
      const issueB = issueIndex.get(edge.target);
      if (!issueA || !issueB) {
        return [];
      }

      const isNeighborLink =
        selectedId != null &&
        ((edge.source === selectedId && neighborIds.has(edge.target)) ||
          (edge.target === selectedId && neighborIds.has(edge.source)));
      const bothHighlighted =
        isHighlighted(edge.source) && isHighlighted(edge.target);
      const similarityRange = Math.max(1 - edgeThreshold, 0.001);
      const normalizedStrength = clamp(
        (edge.similarity - edgeThreshold) / similarityRange,
        0,
        1
      );
      const strength = normalizedStrength ** 1.6;
      const baseOpacity = 0.12 + strength * 0.7;
      const baseWidth = 0.5 + strength * 5;

      return [
        {
          key: `${edge.source}-${edge.target}`,
          ax: issueA.x,
          ay: issueA.y,
          bx: issueB.x,
          by: issueB.y,
          stroke: isNeighborLink ? selectedIssueColor : "currentColor",
          strokeOpacity: isNeighborLink
            ? 0.45 + strength * 0.45
            : hasSelection
              ? bothHighlighted
                ? Math.min(baseOpacity * 1.1, 0.88)
                : baseOpacity * 0.35
              : baseOpacity,
          strokeWidth: isNeighborLink
            ? baseWidth + 1
            : baseWidth,
        },
      ];
    });
  }, [
    edgeThreshold,
    hasSelection,
    issueIndex,
    neighborIds,
    renderedEdges,
    selectedId,
    selectedIssueColor,
    isHighlighted,
  ]);

  const canvasEdges = useMemo<IssueMapCanvasLine[]>(() => {
    return staticEdgeData.map((edge) => {
      const { sx: x1, sy: y1 } = toScreen(edge.ax, edge.ay);
      const { sx: x2, sy: y2 } = toScreen(edge.bx, edge.by);
      return {
        key: edge.key,
        x1,
        y1,
        x2,
        y2,
        stroke: edge.stroke,
        strokeOpacity: edge.strokeOpacity,
        strokeWidth: edge.strokeWidth,
      };
    });
  }, [staticEdgeData, toScreen]);

  const staticNodeData = useMemo(() => {
    return visibleIssues.map((issue) => {
      const color = TAG_COLORS[dominantTag(issue.tags)] ?? "#94a3b8";
      const radius = issueRadius(issue.tags, bubbleSizeTag);
      const isActive = issue.id === hoveredId || issue.id === selectedId;
      const isNeighbor = neighborIds.has(issue.id);
      const highlighted = isHighlighted(issue.id);
      const clusterDimmed = filteredIssueIds != null && !filteredIssueIds.has(issue.id);
      const dimmed = (hasSelection && !highlighted) || clusterDimmed;
      const isClosed = issue.status === "closed";
      const rings: IssueMapCanvasRing[] = [];

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

      if (issue.assignedTo) {
        rings.push({
          radius: radius + 3,
          fill: "none",
          stroke: color,
          strokeWidth: 1.5,
          strokeOpacity: clusterDimmed ? 0.14 : dimmed ? 0.2 : 0.45,
        });
      }

      return {
        id: issue.id,
        x: issue.x,
        y: issue.y,
        radius,
        fill: color,
        fillOpacity: clusterDimmed
          ? 0.08
          : dimmed
            ? 0.16
            : isClosed
              ? isActive
                ? 0.46
                : 0.24
              : isActive
                ? 0.96
                : 0.72,
        filter:
          isActive || selectedBatch.has(issue.id)
            ? `url(#map-node-glow-${canvasThemeID})`
            : undefined,
        stroke: color,
        strokeWidth: isActive ? 2.4 : isClosed ? 1.6 : 1.1,
        strokeOpacity: clusterDimmed ? 0.14 : dimmed ? 0.28 : isClosed ? 0.58 : 0.88,
        strokeDasharray: isClosed ? "4 3" : undefined,
        rings,
        label:
          isActive || (isNeighbor && selectedId != null)
            ? issueLabel(issue.raw, 40)
            : undefined,
        labelYOffset: radius + (isNeighbor && !isActive ? 10 : 8),
        labelClassName: "text-[11px] font-semibold tracking-[0.01em]",
        labelFillOpacity: isNeighbor && !isActive ? 0.6 : 1,
      };
    });
  }, [
    canvasThemeID,
    filteredIssueIds,
    hasSelection,
    hoveredId,
    neighborIds,
    selectedBatch,
    selectedId,
    isHighlighted,
    bubbleSizeTag,
    visibleIssues,
  ]);

  const canvasNodes = useMemo<IssueMapCanvasNode[]>(() => {
    return staticNodeData.map((node) => {
      const { sx, sy } = toScreen(node.x, node.y);
      return {
        key: node.id,
        dataIssue: node.id,
        cx: sx,
        cy: sy,
        radius: node.radius,
        fill: node.fill,
        fillOpacity: node.fillOpacity,
        filter: node.filter,
        stroke: node.stroke,
        strokeWidth: node.strokeWidth,
        strokeOpacity: node.strokeOpacity,
        strokeDasharray: node.strokeDasharray,
        rings: node.rings,
        label: node.label,
        labelY: sy - node.labelYOffset,
        labelClassName: node.labelClassName,
        labelFillOpacity: node.labelFillOpacity,
        className: "cursor-pointer",
      };
    });
  }, [staticNodeData, toScreen]);

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
            <Badge variant="outline" className="tabular-nums">
              {issues.length} issues
            </Badge>
            {closedIssueCount > 0 && (
              <Badge variant="outline" className="tabular-nums">
                {closedIssueCount} closed
              </Badge>
            )}
            <Badge variant="outline" className="tabular-nums">
              {visibleIssues.length} visible
            </Badge>
            <Badge variant="outline" className="tabular-nums">
              {renderedEdges.length} edges
            </Badge>
          </>
        }
        actions={
          <>
            <label className="flex items-center gap-2 text-[11px] text-muted-foreground">
              <Switch
                checked={showClosed}
                onCheckedChange={(checked) => {
                  setShowClosed(checked);
                  setLoadedEdgeDataKey("");
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
                  setLoadedEdgeDataKey("");
                }}
                className="h-1.5 w-28 accent-foreground"
                aria-label="Similarity edge threshold"
              />
            </label>
            <label className="flex items-center gap-2 text-[11px] text-muted-foreground">
              <span>Bubble size</span>
              <select
                value={bubbleSizeTag ?? ""}
                onChange={(event) => {
                  const nextValue = event.target.value.trim();
                  setBubbleSizeTag(nextValue === "" ? null : nextValue);
                }}
                className="h-8 rounded-md border border-border/60 bg-background px-2 text-[11px] text-foreground"
                aria-label="Bubble size tag"
              >
                <option value="">Strongest tag</option>
                {bubbleSizeOptions.map((option) => (
                  <option key={option.tag} value={option.tag}>
                    {option.tag}
                  </option>
                ))}
              </select>
            </label>
            <button
              type="button"
              onClick={() => scheduleViewport(DEFAULT_VIEWPORT)}
              className="text-[11px] text-muted-foreground hover:text-foreground"
            >
              Reset View
            </button>
            {filteredClusterLabel && (
              <button
                type="button"
                onClick={() => setFilteredClusterLabel(null)}
                className="rounded-md bg-primary/10 px-2 py-0.5 text-[11px] font-medium text-primary hover:bg-primary/20"
              >
                ✕ {filteredClusterLabel}
              </button>
            )}
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
              Scroll to zoom. Drag to pan. Shift-click to build a batch. Shift-drag to lasso. Solid halos mark assigned issues. Use Analyze in the sidebar to compare tags and embeddings.
            </span>
          </>
        }
      />
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <div
          ref={containerRef}
          className="app-surface relative min-h-0 flex-1 overflow-hidden overscroll-none"
        >
          <div className="pointer-events-none absolute left-5 top-5 z-10 max-w-sm space-y-3">
            <div className="app-subtle-surface animate-fade-in-up px-4 py-3 shadow-sm">
              <p className="text-[10px] font-semibold uppercase tracking-[0.24em] text-muted-foreground">
                Semantic Projection
              </p>
              <p className="mt-2 text-[13px] leading-relaxed text-foreground/90">
                Nearby issues share stronger structural similarity. Edges add
                text-semantic relationships, and blobs show cluster neighborhoods.
              </p>
              <div className="mt-3 flex flex-wrap gap-2 text-[10px] text-muted-foreground">
                <span className="inline-flex items-center gap-1.5 rounded-full border border-border/50 bg-background/80 px-2 py-1">
                  <span className="h-2.5 w-2.5 rounded-full bg-sky-500/80" />
                  Dominant tag
                </span>
                <span className="inline-flex items-center gap-1.5 rounded-full border border-border/50 bg-background/80 px-2 py-1">
                  <span className="h-3 w-3 rounded-full border border-violet-500/70" />
                  Assigned halo
                </span>
                <span className="inline-flex items-center gap-1.5 rounded-full border border-border/50 bg-background/80 px-2 py-1">
                  <span className="h-3 w-3 rounded-full border border-foreground/40 border-dashed" />
                  Closed issue
                </span>
              </div>
            </div>
          </div>

          <div className="pointer-events-none absolute bottom-5 left-5 z-10 flex max-w-[min(42rem,calc(100%-10rem))] flex-wrap gap-2">
            {visibleTagLegend.map(({ tag, count, color }) => (
              <span
                key={tag}
                className="inline-flex items-center gap-2 rounded-full border border-border/50 bg-background/82 px-3 py-1.5 text-[11px] font-medium text-foreground shadow-sm backdrop-blur-sm"
              >
                <span
                  className="h-2.5 w-2.5 rounded-full"
                  style={{ backgroundColor: color }}
                />
                {tag}
                <span className="text-muted-foreground">{count}</span>
              </span>
            ))}
            <span className="inline-flex items-center rounded-full border border-border/50 bg-background/82 px-3 py-1.5 text-[11px] text-muted-foreground shadow-sm backdrop-blur-sm">
              {selectedBatch.size > 1
                ? `${selectedBatch.size} issues in batch`
                : filteredClusterLabel
                  ? `Filtering ${filteredClusterLabel}`
                  : selectedId
                    ? "Focused issue neighborhood"
                    : `${visibleClusters.length} clusters in view`}
            </span>
          </div>

          <IssueMapCanvas
            ref={svgRef}
            width={dimensions.width}
            height={dimensions.height}
            className="absolute inset-0 touch-none overscroll-none"
            style={{ cursor: isLassoing ? "crosshair" : "default" }}
            background={
              <>
                <defs>
                  <radialGradient id={`map-glow-primary-${canvasThemeID}`} cx="18%" cy="16%" r="68%">
                    <stop offset="0%" stopColor="var(--glow-color)" stopOpacity="0.18" />
                    <stop offset="55%" stopColor="var(--glow-color)" stopOpacity="0.06" />
                    <stop offset="100%" stopColor="var(--glow-color)" stopOpacity="0" />
                  </radialGradient>
                  <radialGradient id={`map-glow-secondary-${canvasThemeID}`} cx="84%" cy="78%" r="62%">
                    <stop offset="0%" stopColor="var(--glow-color-current)" stopOpacity="0.14" />
                    <stop offset="60%" stopColor="var(--glow-color-current)" stopOpacity="0.05" />
                    <stop offset="100%" stopColor="var(--glow-color-current)" stopOpacity="0" />
                  </radialGradient>
                  <pattern
                    id={`map-dot-grid-${canvasThemeID}`}
                    width="30"
                    height="30"
                    patternUnits="userSpaceOnUse"
                  >
                    <circle cx="1.5" cy="1.5" r="1.5" fill="currentColor" fillOpacity="0.065" />
                  </pattern>
                  <filter
                    id={`map-blob-shadow-${canvasThemeID}`}
                    x="-20%"
                    y="-20%"
                    width="140%"
                    height="140%"
                  >
                    <feDropShadow
                      dx="0"
                      dy="14"
                      stdDeviation="18"
                      floodColor="currentColor"
                      floodOpacity="0.08"
                    />
                  </filter>
                  <filter
                    id={`map-node-glow-${canvasThemeID}`}
                    x="-180%"
                    y="-180%"
                    width="360%"
                    height="360%"
                  >
                    <feDropShadow
                      dx="0"
                      dy="0"
                      stdDeviation="8"
                      floodColor="currentColor"
                      floodOpacity="0.18"
                    />
                  </filter>
                </defs>
                <rect
                  x="0"
                  y="0"
                  width={dimensions.width}
                  height={dimensions.height}
                  fill="var(--background)"
                />
                <rect
                  x="0"
                  y="0"
                  width={dimensions.width}
                  height={dimensions.height}
                  fill={`url(#map-glow-primary-${canvasThemeID})`}
                />
                <rect
                  x="0"
                  y="0"
                  width={dimensions.width}
                  height={dimensions.height}
                  fill={`url(#map-glow-secondary-${canvasThemeID})`}
                />
                <rect
                  x={PADDING}
                  y={PADDING}
                  width={Math.max(0, dimensions.width - PADDING * 2)}
                  height={Math.max(0, dimensions.height - PADDING * 2)}
                  rx="22"
                  fill={`url(#map-dot-grid-${canvasThemeID})`}
                  opacity="0.95"
                />
                <rect
                  x={PADDING}
                  y={PADDING}
                  width={Math.max(0, dimensions.width - PADDING * 2)}
                  height={Math.max(0, dimensions.height - PADDING * 2)}
                  rx="22"
                  fill="none"
                  stroke="currentColor"
                  strokeOpacity="0.1"
                />
              </>
            }
            onMouseDown={handleMouseDown}
            onMouseMove={handleMouseMove}
            onMouseUp={handleMouseUp}
            onClick={handleCanvasClick}
            onMouseOver={handleMouseOver}
            onMouseOut={handleMouseOut}
            onMouseLeave={handleMouseUp}
            gridLines={canvasGridLines}
            blobs={canvasBlobs}
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
              sidebarIssue && selectedBatch.size === 0
                ? "translate-x-0"
                : "translate-x-full"
            }`}
          >
            {sidebarIssue && selectedBatch.size === 0 && (
              <div
                data-map-sidebar-scroll="true"
                className="flex h-full flex-col overflow-y-auto overscroll-contain p-5"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="space-y-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge variant="outline" className="px-2 py-0.5 text-[10px]">
                        {sidebarIssue.id}
                      </Badge>
                      <span
                        className={
                          sidebarIssue.status === "closed"
                            ? "rounded-full bg-slate-200 px-2 py-0.5 text-[10px] font-medium text-slate-700"
                            : "rounded-full bg-emerald-100 px-2 py-0.5 text-[10px] font-medium text-emerald-700"
                        }
                      >
                        {sidebarIssue.status === "closed" ? "Closed" : "Open"}
                      </span>
                      {sidebarIssue.assignedTo ? (
                        <span className="rounded-full bg-violet-100 px-2 py-0.5 text-[10px] font-medium text-violet-700">
                          {sidebarIssue.assignedTo}
                        </span>
                      ) : (
                        <span className="rounded-full bg-muted px-2 py-0.5 text-[10px] font-medium text-muted-foreground">
                          Unassigned
                        </span>
                      )}
                    </div>
                    <p className="whitespace-pre-wrap text-[13px] leading-relaxed">
                      {sidebarIssue.raw}
                    </p>
                    <div className="mt-2 flex flex-wrap gap-1.5">
                      {sidebarIssue.tags.map(({ tag }) => (
                        <Link
                          key={tag}
                          href={tagHref(tag)}
                          className="rounded-full px-2 py-0.5 text-[10px] font-medium"
                          style={{
                            backgroundColor: `${TAG_COLORS[tag] ?? "#94a3b8"}20`,
                            color: TAG_COLORS[tag] ?? "#94a3b8",
                          }}
                        >
                          {tag}
                        </Link>
                      ))}
                    </div>
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
                <div className="mt-4">
                  <Link
                    href={`/issues/${sidebarIssue.id}`}
                    className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-[12px] font-medium text-primary-foreground shadow-sm transition-colors hover:bg-primary/90"
                  >
                    View issue
                    <svg
                      width="12"
                      height="12"
                      viewBox="0 0 12 12"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="1.5"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                    >
                      <path d="M4.5 2.5L8.5 6L4.5 9.5" />
                    </svg>
                  </Link>
                </div>
                <div className="mt-5 space-y-2">
                  <TagRelevanceBars
                    tags={sidebarIssue.tags}
                    colorFor={(tag) => TAG_COLORS[tag] ?? "#94a3b8"}
                  />
                  {selectedId === sidebarIssue.id && selectedNeighbors.length > 0 && (
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
              selectedCluster && selectedId == null && selectedBatch.size === 0
                ? "translate-x-0"
                : "translate-x-full"
            }`}
          >
            {selectedCluster && selectedId == null && selectedBatch.size === 0 && (
              <div
                data-map-sidebar-scroll="true"
                className="flex h-full flex-col overflow-y-auto overscroll-contain p-5"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="space-y-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge variant="outline" className="px-2 py-0.5 text-[10px]">Cluster</Badge>
                      <Badge variant="outline" className="px-2 py-0.5 text-[10px]">
                        {selectedClusterIssues.length} issues
                      </Badge>
                      <span className="rounded-full bg-emerald-100 px-2 py-0.5 text-[10px] font-medium text-emerald-700">
                        {selectedClusterOpenCount} open
                      </span>
                      <span className="rounded-full bg-violet-100 px-2 py-0.5 text-[10px] font-medium text-violet-700">
                        {selectedClusterIssues.filter((issue) => Boolean(issue.assignedTo)).length} assigned
                      </span>
                    </div>
                    <p className="text-[15px] font-semibold leading-snug">
                      {selectedCluster.label}
                    </p>
                    <p className="text-[12px] leading-relaxed text-muted-foreground">
                      Blob selection is currently also the cluster filter. Use the issue list below to jump into a specific issue without losing the cluster context.
                    </p>
                  </div>
                  <button
                    type="button"
                    onClick={() => setFilteredClusterLabel(null)}
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
                </div>

                {selectedCluster.topTag && (
                  <div className="mt-4">
                    <Link
                      href={tagHref(selectedCluster.topTag)}
                      className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-[12px] font-medium text-primary-foreground shadow-sm transition-colors hover:bg-primary/90"
                    >
                      View top tag
                      <svg
                        width="12"
                        height="12"
                        viewBox="0 0 12 12"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="1.5"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                      >
                        <path d="M4.5 2.5L8.5 6L4.5 9.5" />
                      </svg>
                    </Link>
                  </div>
                )}

                <div className="mt-5 space-y-2">
                  <TagRelevanceBars
                    tags={selectedClusterTagProfile}
                    colorFor={(tag) => TAG_COLORS[tag] ?? "#94a3b8"}
                  />

                  <div className="mt-5 space-y-1.5">
                    <p className="text-[11px] font-medium text-muted-foreground">
                      Cluster members
                    </p>
                    {selectedClusterIssues.map((issue) => (
                      <IssueListItem
                        key={issue.id}
                        issue={issue}
                        tags={issue.tags}
                        onClick={() => setSelectedId(issue.id)}
                      />
                    ))}
                  </div>
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
              <div
                data-map-sidebar-scroll="true"
                className="flex h-full flex-col overflow-y-auto overscroll-contain p-5"
              >
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
