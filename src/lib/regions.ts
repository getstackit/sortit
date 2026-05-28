import { uiAPIURL } from "@/lib/api";
import { getJSON } from "@/lib/http";

export type RegionKind = "tag" | "cluster" | "custom";

export type RegionKey = {
  kind: RegionKind;
  id: string;
};

export type Region = {
  key: RegionKey;
  label: string;
  description?: string;
};

export type TimeWindow = {
  label: string;
  start: string;
  end: string;
};

export type AgeBucket = {
  label: string;
  count: number;
};

export type Rate = {
  count: number;
  perDay: number;
};

export type RegionMetrics = {
  key: RegionKey;
  window: TimeWindow;
  mass: number;
  massOpen: number;
  massClosed: number;
  ageBuckets?: AgeBucket[];
  density?: number | null;
  growth?: Rate | null;
  closure?: Rate | null;
  churn?: Rate | null;
  authorityLinks?: number;
  authority?: number | null;
};

export type RegionWithMetrics = {
  region: Region;
  metrics: RegionMetrics;
};

export type CorpusOrphans = {
  total: number;
  open: number;
  fraction: number;
};

export type RegionListResponse = {
  regions: RegionWithMetrics[];
  window: TimeWindow;
  orphans?: CorpusOrphans | null;
};

export type RegionGetResponse = {
  region: Region;
  metrics: RegionMetrics;
  window: TimeWindow;
};

export type RegionWindow = "7d" | "30d" | "90d" | "180d" | "365d" | "all";

export const DEFAULT_REGION_WINDOW: RegionWindow = "30d";

export async function fetchRegions(
  kind: RegionKind = "tag",
  window: RegionWindow = DEFAULT_REGION_WINDOW,
  signal?: AbortSignal
): Promise<RegionListResponse> {
  const params = new URLSearchParams({ kind, window });
  return getJSON<RegionListResponse>(uiAPIURL(`/regions?${params.toString()}`), {
    cache: "no-store",
    signal,
  });
}

export async function fetchRegion(
  kind: RegionKind,
  id: string,
  window: RegionWindow = DEFAULT_REGION_WINDOW,
  signal?: AbortSignal
): Promise<RegionGetResponse> {
  const params = new URLSearchParams({ window });
  return getJSON<RegionGetResponse>(
    uiAPIURL(`/regions/${encodeURIComponent(kind)}/${encodeURIComponent(id)}?${params.toString()}`),
    { cache: "no-store", signal }
  );
}
