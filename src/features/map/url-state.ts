import type { MapURLState, Viewport } from "@/features/map/types";
import { clamp } from "@/features/map/model";

export const DEFAULT_VIEWPORT: Viewport = {
  xMin: 0,
  xMax: 1,
  yMin: 0,
  yMax: 1,
};

export const MIN_VIEW_SIZE = 0.12;
export const MAX_VIEW_SIZE = 2.4;
export const PAN_OVERSCAN = 0.35;
export const DEFAULT_EDGE_THRESHOLD = 0.4;

export function clampViewport(viewport: Viewport): Viewport {
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

export function viewportQuery(viewport: Viewport) {
  const params = new URLSearchParams({
    xMin: viewport.xMin.toFixed(4),
    xMax: viewport.xMax.toFixed(4),
    yMin: viewport.yMin.toFixed(4),
    yMax: viewport.yMax.toFixed(4),
  });
  return params.toString();
}

export function mapQuery(
  viewport: Viewport,
  edgeThreshold: number,
  showClosed: boolean
) {
  const params = new URLSearchParams(viewportQuery(viewport));
  params.set("edgeThreshold", edgeThreshold.toFixed(2));
  params.set("status", showClosed ? "all" : "open");
  return params.toString();
}

function parseNumberParam(
  params: Pick<URLSearchParams, "get">,
  key: keyof Viewport
) {
  const raw = params.get(key);
  if (!raw) {
    return null;
  }

  const parsed = Number(raw);
  return Number.isFinite(parsed) ? parsed : null;
}

function parseSelectedIssueID(params: Pick<URLSearchParams, "get">) {
  const value = params.get("issue");
  if (!value) {
    return null;
  }

  const normalized = value.trim();
  return normalized === "" ? null : normalized;
}

function parseBatchIssueIDs(params: Pick<URLSearchParams, "get">) {
  const value = params.get("batch");
  if (!value) {
    return [];
  }

  const seen = new Set<string>();
  const ids: string[] = [];

  for (const raw of value.split(",")) {
    const normalized = raw.trim();
    if (normalized === "" || seen.has(normalized)) {
      continue;
    }

    seen.add(normalized);
    ids.push(normalized);
  }

  return ids;
}

function parseViewportFromParams(params: Pick<URLSearchParams, "get">) {
  const xMin = parseNumberParam(params, "xMin");
  const xMax = parseNumberParam(params, "xMax");
  const yMin = parseNumberParam(params, "yMin");
  const yMax = parseNumberParam(params, "yMax");

  if (xMin == null || xMax == null || yMin == null || yMax == null) {
    return DEFAULT_VIEWPORT;
  }

  return clampViewport({ xMin, xMax, yMin, yMax });
}

function parseEdgeThresholdParam(params: Pick<URLSearchParams, "get">) {
  const raw = params.get("edgeThreshold");
  if (!raw) {
    return DEFAULT_EDGE_THRESHOLD;
  }

  const parsed = Number(raw);
  if (!Number.isFinite(parsed) || parsed < 0 || parsed > 1) {
    return DEFAULT_EDGE_THRESHOLD;
  }

  return parsed;
}

function parseShowClosedParam(params: Pick<URLSearchParams, "get">) {
  return params.get("status") === "all";
}

function parseBubbleSizeTagParam(params: Pick<URLSearchParams, "get">) {
  const value = params.get("sizeTag");
  if (!value) {
    return null;
  }

  const normalized = value.trim();
  return normalized === "" ? null : normalized;
}

export function parseMapURLState(
  params: Pick<URLSearchParams, "get">
): MapURLState {
  const batchIds = parseBatchIssueIDs(params);

  return {
    viewport: parseViewportFromParams(params),
    selectedId: batchIds.length > 0 ? null : parseSelectedIssueID(params),
    batchIds,
    showBatchAnalysis: batchIds.length > 1 && params.get("analyze") === "1",
    edgeThreshold: parseEdgeThresholdParam(params),
    showClosed: parseShowClosedParam(params),
    bubbleSizeTag: parseBubbleSizeTagParam(params),
  };
}
