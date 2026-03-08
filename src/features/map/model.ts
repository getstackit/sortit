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

export function dominantTag(tags: TagRelevance[]): string {
  if (tags.length === 0) {
    return "bug";
  }

  return tags.reduce((left, right) =>
    left.relevance > right.relevance ? left : right
  ).tag;
}

export function issueRadius(tags: TagRelevance[]): number {
  if (tags.length === 0) {
    return 6;
  }

  const maxRelevance = Math.max(...tags.map((tag) => tag.relevance));
  return 6 + maxRelevance * 14;
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
