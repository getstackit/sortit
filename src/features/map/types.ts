export type TagRelevance = {
  tag: string;
  relevance: number;
};

export type MapIssue = {
  id: string;
  raw: string;
  status: "open" | "closed";
  tags: TagRelevance[];
  x: number;
  y: number;
};

export type MapEdge = {
  source: string;
  target: string;
  similarity: number;
};

export type MapCluster = {
  label: string;
  centerX: number;
  centerY: number;
  radius: number;
  color: string;
};

export type MapData = {
  issues: MapIssue[];
  edges: MapEdge[];
  clusters: MapCluster[];
};

export type EdgeData = {
  edges: MapEdge[];
};

export type Viewport = {
  xMin: number;
  xMax: number;
  yMin: number;
  yMax: number;
};

export type ScreenPoint = {
  x: number;
  y: number;
};

export type Neighbor = {
  id: string;
  similarity: number;
};

export type TagComparison = {
  tag: string;
  average: number;
  minimum: number;
  maximum: number;
  spread: number;
};

export type PairwiseTagSimilarity = {
  sourceId: string;
  targetId: string;
  similarity: number;
  sharedTags: TagComparison[];
};

export type PairwiseEmbeddingSimilarity = {
  sourceId: string;
  targetId: string;
  similarity: number;
};

export type BatchEmbeddingAnalysis = {
  comparedIssueCount: number;
  averageEmbeddingSimilarity: number;
  pairwiseEmbeddingSimilarity: PairwiseEmbeddingSimilarity[];
};

export type BatchAnalysis = {
  averageSimilarity: number;
  sharedTags: TagComparison[];
  supportingTags: TagComparison[];
  pairwise: PairwiseTagSimilarity[];
};

export type MapURLState = {
  viewport: Viewport;
  selectedId: string | null;
  batchIds: string[];
  showBatchAnalysis: boolean;
  edgeThreshold: number;
  showClosed: boolean;
};
