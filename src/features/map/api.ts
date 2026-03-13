import { uiAPIURL } from "@/lib/api";
import { getJSON, HTTPError, postJSON } from "@/lib/http";
import type {
  BatchEmbeddingAnalysis,
  EdgeData,
  MapData,
} from "@/features/map/types";

export function fetchMapData(
  query: string,
  signal?: AbortSignal
) {
  return getJSON<MapData>(uiAPIURL(`/map?${query}`), {
    cache: "no-store",
    signal,
  });
}

export async function fetchViewportEdges(
  viewportKey: string,
  signal: AbortSignal
) {
  try {
    return await getJSON<EdgeData>(uiAPIURL(`/map/edges?${viewportKey}`), {
      signal,
    });
  } catch (error) {
    if (!(error instanceof HTTPError) || error.status !== 404) {
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
    uiAPIURL("/issues/compare"),
    { ids },
    { signal }
  );
}
