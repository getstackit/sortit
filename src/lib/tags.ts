import { uiAPIURL } from "@/lib/api";
import { getJSON, postJSON } from "@/lib/http";

export type TagRecord = {
  name: string;
  description?: string;
  createdAt: string;
  embedding: number[];
};

type TagsResponse = {
  tags: TagRecord[];
};

export type DismissedTagMerge = {
  canonicalName: string;
  aliasName: string;
};

export async function fetchTags(signal?: AbortSignal): Promise<TagRecord[]> {
  const payload = await getJSON<TagsResponse>(uiAPIURL("/tags"), {
    cache: "no-store",
    signal,
  });
  return payload.tags;
}

export async function mergeTags(
  canonical: string,
  aliases: string[]
): Promise<void> {
  await postJSON(uiAPIURL("/tags/merge"), { canonical, aliases });
}

export async function dismissTagMerge(
  canonical: string,
  alias: string
): Promise<void> {
  await postJSON(uiAPIURL("/tags/dismiss"), { canonical, alias });
}

export async function fetchDismissedTagMerges(
  signal?: AbortSignal
): Promise<DismissedTagMerge[]> {
  const payload = await getJSON<{ dismissed: DismissedTagMerge[] }>(
    uiAPIURL("/tags/dismissed"),
    { cache: "no-store", signal }
  );
  return payload.dismissed;
}

export function tagHref(name: string): string {
  return `/tags/${encodeURIComponent(name)}`;
}
