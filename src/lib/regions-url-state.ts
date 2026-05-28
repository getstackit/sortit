import { DEFAULT_REGION_WINDOW, type RegionKind, type RegionWindow } from "@/lib/regions";

const SUPPORTED_WINDOWS: readonly RegionWindow[] = [
  "7d",
  "30d",
  "90d",
  "180d",
  "365d",
  "all",
];

export type RegionSortKey = "mass" | "growth" | "closure" | "net" | "name";
const SUPPORTED_SORTS: readonly RegionSortKey[] = [
  "mass",
  "growth",
  "closure",
  "net",
  "name",
];

export type RegionsView = "list" | "map";
const SUPPORTED_VIEWS: readonly RegionsView[] = ["list", "map"];

export type RegionsKindFilter = "tag" | "cluster" | "custom";
const SUPPORTED_KINDS: readonly RegionsKindFilter[] = ["tag", "cluster", "custom"];

export type RegionsURLState = {
  kind: RegionsKindFilter;
  window: RegionWindow;
  view: RegionsView;
  sort: RegionSortKey;
};

export const DEFAULT_REGIONS_URL_STATE: RegionsURLState = {
  kind: "tag",
  window: DEFAULT_REGION_WINDOW,
  view: "list",
  sort: "mass",
};

export function parseRegionsURLState(params: URLSearchParams): RegionsURLState {
  return {
    kind: pick(params.get("kind"), SUPPORTED_KINDS, DEFAULT_REGIONS_URL_STATE.kind),
    window: pick(params.get("window"), SUPPORTED_WINDOWS, DEFAULT_REGIONS_URL_STATE.window),
    view: pick(params.get("view"), SUPPORTED_VIEWS, DEFAULT_REGIONS_URL_STATE.view),
    sort: pick(params.get("sort"), SUPPORTED_SORTS, DEFAULT_REGIONS_URL_STATE.sort),
  };
}

export function regionsURLQuery(state: RegionsURLState): string {
  const params = new URLSearchParams();
  if (state.kind !== DEFAULT_REGIONS_URL_STATE.kind) {
    params.set("kind", state.kind);
  }
  if (state.window !== DEFAULT_REGIONS_URL_STATE.window) {
    params.set("window", state.window);
  }
  if (state.view !== DEFAULT_REGIONS_URL_STATE.view) {
    params.set("view", state.view);
  }
  if (state.sort !== DEFAULT_REGIONS_URL_STATE.sort) {
    params.set("sort", state.sort);
  }
  return params.toString();
}

export function asRegionKind(filter: RegionsKindFilter): RegionKind {
  return filter;
}

function pick<T extends string>(
  raw: string | null,
  allowed: readonly T[],
  fallback: T
): T {
  if (raw && (allowed as readonly string[]).includes(raw)) {
    return raw as T;
  }
  return fallback;
}
