import useSWR from "swr";

import {
  DEFAULT_REGION_WINDOW,
  fetchRegions,
  type RegionKind,
  type RegionListResponse,
  type RegionWindow,
} from "@/lib/regions";

export function useRegions(
  kind: RegionKind = "tag",
  window: RegionWindow = DEFAULT_REGION_WINDOW
) {
  return useSWR<RegionListResponse, Error>(
    ["regions", kind, window] as const,
    () => fetchRegions(kind, window)
  );
}
