import { uiAPIURL } from "@/lib/api";
import { getJSON } from "@/lib/http";
import type { SearchIssueRecord, IssueTagScore } from "@/lib/issues";

export type RelatedTag = {
  name: string;
  description: string;
  similarity: number;
};

export type UnifiedSearchQuery = {
  raw: string;
  tags: IssueTagScore[];
};

export type UnifiedSearchResponse = {
  query: UnifiedSearchQuery;
  issues: SearchIssueRecord[];
  relatedTags: RelatedTag[];
};

type UnifiedSearchOptions = {
  limit?: number;
};

export async function searchUnified(
  query: string,
  options: UnifiedSearchOptions = {},
  signal?: AbortSignal
): Promise<UnifiedSearchResponse> {
  const params = new URLSearchParams();
  params.set("q", query);
  if (options.limit) {
    params.set("limit", String(options.limit));
  }

  return getJSON<UnifiedSearchResponse>(
    uiAPIURL(`/search?${params.toString()}`),
    {
      cache: "no-store",
      signal,
    }
  );
}
