import { apiURL } from "@/lib/api";
import { getJSON, postJSON } from "@/lib/http";
import type {
  BatchEmbeddingAnalysis,
  EdgeData,
  MapData,
} from "@/features/map/types";

export function fetchMapData(
  query: string,
  signal?: AbortSignal
) {
  return getJSON<MapData>(apiURL(`/api/v1/map?${query}`), {
    cache: "no-store",
    signal,
  });
}

export async function fetchViewportEdges(
  viewportKey: string,
  signal: AbortSignal
) {
  try {
    return await getJSON<EdgeData>(apiURL(`/api/v1/map/edges?${viewportKey}`), {
      signal,
    });
  } catch (error) {
    if (!(error instanceof Error) || error.message !== "Request failed with 404") {
      throw error;
    }

    const fallback = await fetchMapData(viewportKey, signal);
    return { edges: fallback.edges };
  }
}

export function compareIssueEmbeddings(
  ids: string[],
  signal: AbortSignal
) {
  return postJSON<BatchEmbeddingAnalysis, { ids: string[] }>(
    apiURL("/api/v1/issues/compare"),
    { ids },
    { signal }
  );
}
