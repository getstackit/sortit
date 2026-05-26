import useSWR from "swr";

import {
  DEFAULT_REGION_WINDOW,
  fetchRegion,
  type RegionGetResponse,
  type RegionKind,
  type RegionWindow,
} from "@/lib/regions";
import { HTTPError } from "@/lib/http";

export function useRegion(
  kind: RegionKind | null,
  id: string | null,
  window: RegionWindow = DEFAULT_REGION_WINDOW
) {
  const key = kind && id ? (["region", kind, id, window] as const) : null;
  return useSWR<RegionGetResponse, Error>(
    key,
    async () => {
      try {
        return await fetchRegion(kind!, id!, window);
      } catch (error) {
        // A 404 means the region has no member issues yet — surface as null so
        // consumers can render a friendly empty state instead of a global error.
        if (error instanceof HTTPError && error.status === 404) {
          return {
            region: { key: { kind: kind!, id: id! }, label: id! },
            metrics: {
              key: { kind: kind!, id: id! },
              window: { label: window, start: "", end: "" },
              mass: 0,
              massOpen: 0,
              massClosed: 0,
            },
            window: { label: window, start: "", end: "" },
          };
        }
        throw error;
      }
    }
  );
}
