import { apiURL } from "@/lib/api";
import { getJSON } from "@/lib/http";

export type TagRecord = {
  name: string;
  description?: string;
  createdAt: string;
  embedding: number[];
};

type TagsResponse = {
  tags: TagRecord[];
};

export async function fetchTags(signal?: AbortSignal): Promise<TagRecord[]> {
  const payload = await getJSON<TagsResponse>(apiURL("/api/v1/tags"), {
    cache: "no-store",
    signal,
  });
  return payload.tags;
}
