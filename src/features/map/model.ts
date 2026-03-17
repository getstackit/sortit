import type {
  BatchAnalysis,
  MapCluster,
  MapIssue,
  PairwiseTagSimilarity,
  ScreenPoint,
  TagRelevance,
  Viewport,
} from "@/features/map/types";

export const TAG_COLORS: Record<string, string> = {
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

export const MIN_RENDERED_AMBIENT_EDGES = 24;
export const RENDERED_EDGE_RATIO = 0.2;
export const MAX_RENDERED_AMBIENT_EDGES = 180;
export const MAX_RENDERED_SELECTED_EDGES = 40;
export const DEFAULT_ISSUE_RADIUS = 6;
export const ISSUE_RADIUS_SCALE = 14;
export const HUBNESS_RADIUS_SCALE = 4;

export function dominantTag(tags: TagRelevance[]): string {
  if (tags.length === 0) {
    return "bug";
  }

  return tags.reduce((left, right) =>
    left.relevance > right.relevance ? left : right
  ).tag;
}

export function issueLoading(
  tags: TagRelevance[],
  bubbleSizeTag?: string | null
) {
  if (tags.length === 0) {
    return 0;
  }

  if (!bubbleSizeTag) {
    return Math.max(...tags.map((tag) => tag.relevance));
  }

  return tags.find((tag) => tag.tag === bubbleSizeTag)?.relevance ?? 0;
}

export function issueRadius(
  tags: TagRelevance[],
  bubbleSizeTag?: string | null,
  hubness?: number
): number {
  return (
    DEFAULT_ISSUE_RADIUS +
    issueLoading(tags, bubbleSizeTag) * ISSUE_RADIUS_SCALE +
    (hubness ?? 0) * HUBNESS_RADIUS_SCALE
  );
}

export function pointInPolygon(
  px: number,
  py: number,
  polygon: ScreenPoint[]
) {
  let inside = false;

  for (let index = 0, previous = polygon.length - 1; index < polygon.length; previous = index++) {
    const currentPoint = polygon[index];
    const previousPoint = polygon[previous];

    if (
      (currentPoint.y > py) !== (previousPoint.y > py) &&
      px <
        ((previousPoint.x - currentPoint.x) * (py - currentPoint.y)) /
          (previousPoint.y - currentPoint.y) +
          currentPoint.x
    ) {
      inside = !inside;
    }
  }

  return inside;
}

export function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

export function normalizeWheelDelta(
  delta: number,
  deltaMode: number,
  pageHeight: number
) {
  if (deltaMode === 1) {
    return delta * 18;
  }

  if (deltaMode === 2) {
    return delta * pageHeight;
  }

  return delta;
}

export function edgeRenderLimit(visibleNodeCount: number) {
  if (visibleNodeCount <= 0) {
    return 0;
  }

  return Math.min(
    MAX_RENDERED_AMBIENT_EDGES,
    Math.max(
      MIN_RENDERED_AMBIENT_EDGES,
      Math.ceil(visibleNodeCount * RENDERED_EDGE_RATIO)
    )
  );
}

export function clusterIntersectsViewport(
  cluster: MapCluster,
  viewport: Viewport
) {
  return !(
    cluster.centerX + cluster.radius < viewport.xMin ||
    cluster.centerX - cluster.radius > viewport.xMax ||
    cluster.centerY + cluster.radius < viewport.yMin ||
    cluster.centerY - cluster.radius > viewport.yMax
  );
}

function tagRelevanceMap(tags: TagRelevance[]) {
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
    const leftValue = left[tag] ?? 0;
    const rightValue = right[tag] ?? 0;
    dot += leftValue * rightValue;
    leftMagnitude += leftValue * leftValue;
    rightMagnitude += rightValue * rightValue;
  }

  if (leftMagnitude === 0 || rightMagnitude === 0) {
    return 0;
  }

  return dot / (Math.sqrt(leftMagnitude) * Math.sqrt(rightMagnitude));
}

export function analyzeBatch(batchIssues: MapIssue[]): BatchAnalysis | null {
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

export function computeConvexHull(points: { x: number; y: number }[]) {
  const sorted = [...points].sort((a, b) => a.x - b.x || a.y - b.y);
  if (sorted.length <= 1) return sorted;

  function cross(o: { x: number; y: number }, a: { x: number; y: number }, b: { x: number; y: number }) {
    return (a.x - o.x) * (b.y - o.y) - (a.y - o.y) * (b.x - o.x);
  }

  const lower: { x: number; y: number }[] = [];
  for (const p of sorted) {
    while (lower.length >= 2 && cross(lower[lower.length - 2], lower[lower.length - 1], p) <= 0) {
      lower.pop();
    }
    lower.push(p);
  }

  const upper: { x: number; y: number }[] = [];
  for (let i = sorted.length - 1; i >= 0; i--) {
    const p = sorted[i];
    while (upper.length >= 2 && cross(upper[upper.length - 2], upper[upper.length - 1], p) <= 0) {
      upper.pop();
    }
    upper.push(p);
  }

  lower.pop();
  upper.pop();
  return lower.concat(upper);
}

/** Chaikin corner-cutting on a closed polygon. Each iteration replaces every
 *  edge with two points at the 25% and 75% marks, rounding off sharp corners. */
function chaikinSmooth(
  pts: { x: number; y: number }[],
  iterations: number,
): { x: number; y: number }[] {
  let cur = pts;
  for (let iter = 0; iter < iterations; iter++) {
    const next: { x: number; y: number }[] = [];
    for (let i = 0; i < cur.length; i++) {
      const a = cur[i];
      const b = cur[(i + 1) % cur.length];
      next.push({ x: 0.75 * a.x + 0.25 * b.x, y: 0.75 * a.y + 0.25 * b.y });
      next.push({ x: 0.25 * a.x + 0.75 * b.x, y: 0.25 * a.y + 0.75 * b.y });
    }
    cur = next;
  }
  return cur;
}

export function computeBlobPath(screenPoints: { x: number; y: number }[], padding: number): string {
  if (screenPoints.length < 3) return "";

  const hull = computeConvexHull(screenPoints);
  if (hull.length < 3) return "";

  const cx = hull.reduce((s, p) => s + p.x, 0) / hull.length;
  const cy = hull.reduce((s, p) => s + p.y, 0) / hull.length;

  const expanded = hull.map((p) => {
    const dx = p.x - cx;
    const dy = p.y - cy;
    const dist = Math.sqrt(dx * dx + dy * dy) || 1;
    return {
      x: p.x + (dx / dist) * padding,
      y: p.y + (dy / dist) * padding,
    };
  });

  // Chaikin corner-cutting: 3 iterations to smooth out kinks and spikes
  const smoothed = chaikinSmooth(expanded, 3);

  const n = smoothed.length;
  const parts: string[] = [];

  for (let i = 0; i < n; i++) {
    const p0 = smoothed[(i - 1 + n) % n];
    const p1 = smoothed[i];
    const p2 = smoothed[(i + 1) % n];
    const p3 = smoothed[(i + 2) % n];

    const cp1x = p1.x + (p2.x - p0.x) / 6;
    const cp1y = p1.y + (p2.y - p0.y) / 6;
    const cp2x = p2.x - (p3.x - p1.x) / 6;
    const cp2y = p2.y - (p3.y - p1.y) / 6;

    if (i === 0) {
      parts.push(`M ${p1.x} ${p1.y}`);
    }

    parts.push(`C ${cp1x} ${cp1y}, ${cp2x} ${cp2y}, ${p2.x} ${p2.y}`);
  }

  parts.push("Z");
  return parts.join(" ");
}

const BLOB_COLORS = [
  "#a78bfa",
  "#60a5fa",
  "#34d399",
  "#fbbf24",
  "#f87171",
];

export function hashTagColor(tag: string): string {
  let sum = 0;
  for (let i = 0; i < tag.length; i++) {
    sum += tag.charCodeAt(i);
  }
  return BLOB_COLORS[sum % BLOB_COLORS.length];
}

export function uniqueBlobColors(keys: readonly string[]): Record<string, string> {
  const colors: Record<string, string> = {};
  const usedHues = new Set<number>();

  for (const key of [...new Set(keys)].sort()) {
    let hue = 0;
    for (let index = 0; index < key.length; index += 1) {
      hue = (hue + key.charCodeAt(index) * (index + 1)) % 360;
    }

    while (usedHues.has(hue)) {
      hue = (hue + 37) % 360;
    }

    usedHues.add(hue);
    colors[key] = `hsl(${hue} 78% 63%)`;
  }

  return colors;
}
