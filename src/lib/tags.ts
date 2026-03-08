import { apiURL } from "@/lib/api";

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
  const response = await fetch(apiURL("/api/v1/tags"), {
    cache: "no-store",
    signal,
  });

  if (!response.ok) {
    throw new Error(`Request failed with ${response.status}`);
  }

  const payload = (await response.json()) as TagsResponse;
  return payload.tags;
}
