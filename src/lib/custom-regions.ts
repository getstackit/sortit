import { uiAPIURL } from "@/lib/api";
import { deleteJSON, getJSON, postJSON } from "@/lib/http";

export type TagPredicate = {
  tag: string;
  minRelevance: number;
};

export type CustomRegionPredicate = {
  tags?: TagPredicate[];
  statuses?: string[];
  authors?: string[];
  maxAgeDays?: number | null;
};

export type CustomRegion = {
  id: string;
  label: string;
  description?: string;
  definition: CustomRegionPredicate;
  createdBy?: string;
  createdAt: string;
  updatedAt: string;
};

export type CustomRegionInput = {
  id?: string;
  label: string;
  description?: string;
  definition: CustomRegionPredicate;
};

export async function getCustomRegion(id: string, signal?: AbortSignal): Promise<CustomRegion> {
  return getJSON<CustomRegion>(
    uiAPIURL(`/regions/custom/${encodeURIComponent(id)}/definition`),
    { cache: "no-store", signal },
  );
}

export async function createCustomRegion(input: CustomRegionInput): Promise<CustomRegion> {
  return postJSON<CustomRegion, CustomRegionInput>(
    uiAPIURL("/regions/custom"),
    input,
  );
}

export async function deleteCustomRegion(id: string): Promise<void> {
  await deleteJSON(uiAPIURL(`/regions/custom/${encodeURIComponent(id)}`));
}
